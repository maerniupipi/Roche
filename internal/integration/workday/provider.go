package workday

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
	secutils "roche.local/knowledge-agent-platform/internal/utils"
)

func NewProvider(cfg *config.Config) (interfaces.WorkdayProvider, error) {
	if cfg == nil || cfg.Workday == nil || !cfg.Workday.Enable {
		return &disabledProvider{}, nil
	}
	switch cfg.Workday.Provider {
	case "mock":
		return newMockProvider(cfg.Workday.MockFile)
	case "http":
		return newHTTPProvider(cfg.Workday)
	default:
		return nil, fmt.Errorf("unsupported Workday provider %q", cfg.Workday.Provider)
	}
}

type disabledProvider struct{}

func (*disabledProvider) Name() string { return "disabled" }
func (*disabledProvider) FetchOrgUnits(context.Context, string, int) (*types.WorkdayOrgUnitPage, error) {
	return nil, errors.New("Workday integration is disabled")
}
func (*disabledProvider) FetchWorkers(context.Context, string, int) (*types.WorkdayWorkerPage, error) {
	return nil, errors.New("Workday integration is disabled")
}

type mockFixture struct {
	OrgUnits         []types.WorkdayOrgUnitRecord `json:"org_units"`
	Workers          []types.WorkdayWorkerRecord  `json:"workers"`
	GeneratedWorkers *mockWorkerGenerator         `json:"generated_workers,omitempty"`
}

type mockWorkerGenerator struct {
	Count             int      `json:"count"`
	UsernamePrefix    string   `json:"username_prefix"`
	EmailDomain       string   `json:"email_domain"`
	ExternalIDPrefix  string   `json:"external_id_prefix"`
	OrgExternalIDs    []string `json:"org_external_ids"`
	ManagerExternalID string   `json:"manager_external_worker_id,omitempty"`
}

type mockProvider struct {
	fixture mockFixture
}

func newMockProvider(path string) (*mockProvider, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Workday mock file: %w", err)
	}
	var fixture mockFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return nil, fmt.Errorf("decode Workday mock file: %w", err)
	}
	if err := fixture.expandGeneratedWorkers(); err != nil {
		return nil, fmt.Errorf("generate Workday mock workers: %w", err)
	}
	return &mockProvider{fixture: fixture}, nil
}

func (f *mockFixture) expandGeneratedWorkers() error {
	generator := f.GeneratedWorkers
	if generator == nil || generator.Count == 0 {
		return nil
	}
	if generator.Count < 0 || generator.Count > 10000 {
		return errors.New("generated_workers.count must be between 0 and 10000")
	}
	usernamePrefix := strings.TrimSpace(generator.UsernamePrefix)
	emailDomain := strings.TrimSpace(generator.EmailDomain)
	externalIDPrefix := strings.TrimSpace(generator.ExternalIDPrefix)
	if usernamePrefix == "" || emailDomain == "" || externalIDPrefix == "" {
		return errors.New("generated_workers username_prefix, email_domain and external_id_prefix are required")
	}
	if len(generator.OrgExternalIDs) == 0 {
		return errors.New("generated_workers.org_external_ids must not be empty")
	}

	workers := make([]types.WorkdayWorkerRecord, 0, len(f.Workers)+generator.Count)
	workers = append(workers, f.Workers...)
	for i := 1; i <= generator.Count; i++ {
		sequence := fmt.Sprintf("%03d", i)
		username := usernamePrefix + sequence
		workers = append(workers, types.WorkdayWorkerRecord{
			ExternalID:              externalIDPrefix + sequence,
			PrimaryOrgExternalID:    generator.OrgExternalIDs[(i-1)%len(generator.OrgExternalIDs)],
			ManagerExternalWorkerID: strings.TrimSpace(generator.ManagerExternalID),
			CorporateEmail:          username + "@" + emailDomain,
			Status:                  types.ExternalWorkerActive,
			Attributes: map[string]any{
				"display_name": "Mock Developer " + sequence,
			},
		})
	}
	f.Workers = workers
	return nil
}

func (*mockProvider) Name() string { return "workday" }

func (p *mockProvider) FetchOrgUnits(
	_ context.Context,
	cursor string,
	pageSize int,
) (*types.WorkdayOrgUnitPage, error) {
	start, err := parseMockCursor(cursor)
	if err != nil {
		return nil, err
	}
	end, next := pageBounds(start, pageSize, len(p.fixture.OrgUnits))
	return &types.WorkdayOrgUnitPage{
		Items:      append([]types.WorkdayOrgUnitRecord(nil), p.fixture.OrgUnits[start:end]...),
		NextCursor: next,
	}, nil
}

func (p *mockProvider) FetchWorkers(
	_ context.Context,
	cursor string,
	pageSize int,
) (*types.WorkdayWorkerPage, error) {
	start, err := parseMockCursor(cursor)
	if err != nil {
		return nil, err
	}
	end, next := pageBounds(start, pageSize, len(p.fixture.Workers))
	return &types.WorkdayWorkerPage{
		Items:      append([]types.WorkdayWorkerRecord(nil), p.fixture.Workers[start:end]...),
		NextCursor: next,
	}, nil
}

func parseMockCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(cursor)
	if err != nil || offset < 0 {
		return 0, fmt.Errorf("invalid Workday mock cursor %q", cursor)
	}
	return offset, nil
}

func pageBounds(start, pageSize, length int) (int, string) {
	if start > length {
		start = length
	}
	if pageSize <= 0 {
		pageSize = 200
	}
	end := start + pageSize
	if end >= length {
		return length, ""
	}
	return end, strconv.Itoa(end)
}

type httpProvider struct {
	cfg        *config.WorkdayConfig
	client     *http.Client
	tokenMu    sync.Mutex
	token      string
	tokenUntil time.Time
}

type clientCredentialsResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

func newHTTPProvider(cfg *config.WorkdayConfig) (*httpProvider, error) {
	switch cfg.Pagination {
	case "", "cursor", "offset":
		// supported
	default:
		return nil, fmt.Errorf("unsupported Workday pagination %q (want \"cursor\" or \"offset\")", cfg.Pagination)
	}
	for label, raw := range map[string]string{
		"base URL":  cfg.BaseURL,
		"token URL": cfg.TokenURL,
	} {
		if raw == "" {
			continue
		}
		if err := secutils.ValidateURLForSSRF(raw); err != nil {
			return nil, fmt.Errorf("Workday %s failed SSRF validation: %w", label, err)
		}
	}
	clientCfg := secutils.DefaultSSRFSafeHTTPClientConfig()
	clientCfg.Timeout = cfg.RequestTimeout
	return &httpProvider{
		cfg:    cfg,
		client: secutils.NewSSRFSafeHTTPClient(clientCfg),
	}, nil
}

func (*httpProvider) Name() string { return "workday" }

func (p *httpProvider) FetchOrgUnits(
	ctx context.Context,
	cursor string,
	pageSize int,
) (*types.WorkdayOrgUnitPage, error) {
	var page types.WorkdayOrgUnitPage
	if err := p.fetchPage(ctx, p.cfg.OrgUnitsPath, cursor, pageSize, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

func (p *httpProvider) FetchWorkers(
	ctx context.Context,
	cursor string,
	pageSize int,
) (*types.WorkdayWorkerPage, error) {
	var page types.WorkdayWorkerPage
	if err := p.fetchPage(ctx, p.cfg.WorkersPath, cursor, pageSize, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

func (p *httpProvider) fetchPage(
	ctx context.Context,
	path, cursor string,
	pageSize int,
	target any,
) error {
	endpoint, err := url.JoinPath(strings.TrimRight(p.cfg.BaseURL, "/"), path)
	if err != nil {
		return fmt.Errorf("build Workday request URL: %w", err)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("parse Workday request URL: %w", err)
	}
	query := parsed.Query()
	switch p.cfg.Pagination {
	case "offset":
		// Roche employee API style: GET /employees?offset=1&limit=100
		query.Set("offset", strconv.Itoa(parseOffset(cursor)))
		query.Set("limit", strconv.Itoa(pageSize))
	default:
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		query.Set("page_size", strconv.Itoa(pageSize))
	}
	parsed.RawQuery = query.Encode()
	if err := secutils.ValidateURLForSSRF(parsed.String()); err != nil {
		return fmt.Errorf("Workday request failed SSRF validation: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if p.cfg.TokenURL != "" {
		token, tokenErr := p.accessToken(ctx)
		if tokenErr != nil {
			return tokenErr
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("call Workday adapter: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Workday adapter returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("read Workday adapter response: %w", err)
	}
	if p.cfg.Pagination == "offset" {
		return p.decodeOffsetPage(body, cursor, pageSize, target)
	}
	if page, ok := target.(*types.WorkdayWorkerPage); ok {
		if items, matched := decodeRocheEmployees(body); matched {
			page.Items = items
			return nil
		}
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode Workday adapter response: %w", err)
	}
	return nil
}

// parseOffset converts the opaque cursor to an offset. The Roche employee
// API starts at offset=1, so an empty cursor maps to 1.
func parseOffset(cursor string) int {
	offset, err := strconv.Atoi(cursor)
	if err != nil || offset < 1 {
		return 1
	}
	return offset
}

// nextOffsetCursor advances the offset when a full page was returned; an
// empty string signals the end of the stream.
func nextOffsetCursor(offset, returned, pageSize int) string {
	if returned <= 0 || returned < pageSize {
		return ""
	}
	return strconv.Itoa(offset + returned)
}

// decodeOffsetPage accepts { "employees": [...] } (Roche employee API),
// { "items": [...] } and bare [...] payloads, fills the typed page slice and
// computes the next offset cursor.
func (p *httpProvider) decodeOffsetPage(body []byte, cursor string, pageSize int, target any) error {
	var container struct {
		Items json.RawMessage `json:"items"`
	}
	rawItems := json.RawMessage(body)
	if err := json.Unmarshal(body, &container); err == nil && len(container.Items) > 0 {
		rawItems = container.Items
	}
	offset := parseOffset(cursor)
	switch t := target.(type) {
	case *types.WorkdayWorkerPage:
		// The Roche employee API returns {"employees": [...]} with a nested
		// shape; convert it to the provider-neutral worker contract before
		// falling back to the Workday-native item shapes.
		if items, matched := decodeRocheEmployees(body); matched {
			t.Items = items
			t.NextCursor = nextOffsetCursor(offset, len(items), pageSize)
			return nil
		}
		if err := json.Unmarshal(rawItems, &t.Items); err != nil {
			return fmt.Errorf("decode Workday workers response: %w", err)
		}
		t.NextCursor = nextOffsetCursor(offset, len(t.Items), pageSize)
	case *types.WorkdayOrgUnitPage:
		if err := json.Unmarshal(rawItems, &t.Items); err != nil {
			return fmt.Errorf("decode Workday org units response: %w", err)
		}
		t.NextCursor = nextOffsetCursor(offset, len(t.Items), pageSize)
	default:
		return fmt.Errorf("unsupported Workday page target %T", target)
	}
	return nil
}

// rocheEmployeeEnvelope mirrors the Roche employee API response shape
// ({"employees": [...]}), see employees.json fixtures.
type rocheEmployeeEnvelope struct {
	Employees []rocheEmployee `json:"employees"`
}

// rocheEmployee captures the fields of the Roche employee API that are
// relevant for directory synchronization. Unknown fields are ignored.
type rocheEmployee struct {
	EmployeeWID   string `json:"employeeWId"`
	EmployeeID    string `json:"employeeId"`
	PersistentID  string `json:"persistentId"`
	UserID        string `json:"userId"`
	LastModified  string `json:"lastModifiedOn"`
	Status        struct {
		IsActive bool `json:"isActive"`
	} `json:"status"`
	Organization struct {
		Supervisory struct {
			ID   string `json:"id"`
			Code string `json:"code"`
			Name string `json:"name"`
			Manager struct {
				WID string `json:"WId"`
			} `json:"manager"`
		} `json:"supervisory"`
	} `json:"organization"`
	PreferredName struct {
		FormattedName string `json:"formattedName"`
		ReportingName string `json:"reportingName"`
		FirstName     string `json:"firstName"`
		LastName      string `json:"lastName"`
	} `json:"preferredName"`
	ContactInformation struct {
		WorkContact struct {
			Emails []struct {
				IsPrimary    bool   `json:"isPrimary"`
				EmailAddress string `json:"emailAddress"`
			} `json:"emails"`
		} `json:"workContact"`
	} `json:"contactInformation"`
}

// decodeRocheEmployees reports whether body is a Roche employee API page and,
// when it is, converts the employees into the provider-neutral worker contract.
func decodeRocheEmployees(body []byte) ([]types.WorkdayWorkerRecord, bool) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(body, &probe); err != nil || probe["employees"] == nil {
		return nil, false
	}
	var envelope rocheEmployeeEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, false
	}
	items := make([]types.WorkdayWorkerRecord, 0, len(envelope.Employees))
	for i := range envelope.Employees {
		items = append(items, convertRocheEmployee(&envelope.Employees[i]))
	}
	return items, true
}

// convertRocheEmployee maps one Roche employee to the provider-neutral
// WorkdayWorkerRecord consumed by the synchronization layer.
func convertRocheEmployee(e *rocheEmployee) types.WorkdayWorkerRecord {
	status := types.ExternalWorkerInactive
	if e.Status.IsActive {
		status = types.ExternalWorkerActive
	}
	corporateEmail := ""
	for _, email := range e.ContactInformation.WorkContact.Emails {
		if strings.TrimSpace(email.EmailAddress) == "" {
			continue
		}
		corporateEmail = email.EmailAddress
		if email.IsPrimary {
			break
		}
	}
	displayName := strings.TrimSpace(e.PreferredName.FormattedName)
	if displayName == "" {
		parts := []string{strings.TrimSpace(e.PreferredName.FirstName), strings.TrimSpace(e.PreferredName.LastName)}
		displayName = strings.TrimSpace(strings.Join(parts, " "))
	}
	attributes := map[string]any{}
	setRocheAttribute(attributes, "employee_id", e.EmployeeID)
	setRocheAttribute(attributes, "persistent_id", e.PersistentID)
	setRocheAttribute(attributes, "user_id", e.UserID)
	setRocheAttribute(attributes, "display_name", displayName)
	setRocheAttribute(attributes, "first_name", e.PreferredName.FirstName)
	setRocheAttribute(attributes, "last_name", e.PreferredName.LastName)
	setRocheAttribute(attributes, "reporting_name", e.PreferredName.ReportingName)
	setRocheAttribute(attributes, "supervisory_id", e.Organization.Supervisory.ID)
	setRocheAttribute(attributes, "supervisory_code", e.Organization.Supervisory.Code)
	setRocheAttribute(attributes, "supervisory_name", e.Organization.Supervisory.Name)
	setRocheAttribute(attributes, "last_modified_on", e.LastModified)
	return types.WorkdayWorkerRecord{
		ExternalID:              strings.TrimSpace(e.EmployeeWID),
		PrimaryOrgExternalID:    strings.TrimSpace(e.Organization.Supervisory.ID),
		ManagerExternalWorkerID: strings.TrimSpace(e.Organization.Supervisory.Manager.WID),
		CorporateEmail:          strings.TrimSpace(corporateEmail),
		Status:                  status,
		Attributes:              attributes,
	}
}

func setRocheAttribute(attributes map[string]any, key, value string) {
	if strings.TrimSpace(value) != "" {
		attributes[key] = value
	}
}

func (p *httpProvider) accessToken(ctx context.Context) (string, error) {
	p.tokenMu.Lock()
	defer p.tokenMu.Unlock()
	if p.token != "" && time.Until(p.tokenUntil) > time.Minute {
		return p.token, nil
	}
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	if p.cfg.Scope != "" {
		form.Set("scope", p.cfg.Scope)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.cfg.TokenURL,
		bytes.NewBufferString(form.Encode()),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(p.cfg.ClientID, p.cfg.ClientSecret)
	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request Workday access token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("Workday token endpoint returned status %d", resp.StatusCode)
	}
	var token clientCredentialsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&token); err != nil {
		return "", fmt.Errorf("decode Workday access token: %w", err)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return "", errors.New("Workday token response did not contain access_token")
	}
	p.token = token.AccessToken
	if token.ExpiresIn <= 0 {
		token.ExpiresIn = 300
	}
	p.tokenUntil = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	return p.token, nil
}

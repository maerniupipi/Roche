// Command workday-check validates connectivity to the Workday/MuleSoft
// adapter using the same configuration and provider used by the server,
// WITHOUT writing anything to the database and WITHOUT triggering a sync.
//
// It is intended to answer one question before enabling the sync: "can we
// get a token with the configured Client ID/Secret and pull workers?"
//
// Usage (from the rochekap directory, with the same env as the server):
//
//	go run ./cmd/workday-check [-limit N] [-org] [-raw]
//
// Flags:
//
//	-limit N   number of workers to fetch for validation (default 5)
//	-org       also fetch one page of org units
//	-raw       diagnostic mode: print raw HTTP status + response body of the
//	           token endpoint and the data endpoint (useful for 401/403/404)
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/integration/workday"
	"roche.local/knowledge-agent-platform/internal/types"
	secutils "roche.local/knowledge-agent-platform/internal/utils"
)

func main() {
	var (
		limit = flag.Int("limit", 5, "number of workers to fetch for validation")
		orgs  = flag.Bool("org", false, "also fetch one page of org units")
		raw   = flag.Bool("raw", false, "print raw HTTP status/body of token and data endpoints")
	)
	flag.Parse()
	if *limit <= 0 {
		*limit = 5
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(2)
	}
	wd := cfg.Workday
	if wd == nil {
		fmt.Fprintln(os.Stderr, "workday section is missing from config; set WORKDAY_ENABLE etc. in the environment")
		os.Exit(2)
	}
	if !wd.Enable {
		fmt.Fprintln(os.Stderr, "WORKDAY_ENABLE is false — set it to true (and WORKDAY_PROVIDER=http) before validating")
		os.Exit(2)
	}
	if wd.Provider != "http" {
		fmt.Fprintf(os.Stderr, "WORKDAY_PROVIDER=%q — validation requires the real http provider (WORKDAY_PROVIDER=http)\n", wd.Provider)
		os.Exit(2)
	}

	printConfigSummary(wd)

	// Fail fast on missing essential settings so the provider error below
	// is not confusing.
	missing := []string{}
	if wd.BaseURL == "" {
		missing = append(missing, "WORKDAY_BASE_URL")
	}
	if wd.WorkersPath == "" {
		missing = append(missing, "WORKDAY_WORKERS_PATH")
	}
	if wd.TokenURL == "" {
		fmt.Fprintln(os.Stderr, "warning: WORKDAY_TOKEN_URL is empty — the data endpoint will be called without a Bearer token")
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "missing required config: %s\n", strings.Join(missing, ", "))
		os.Exit(2)
	}

	// Diagnostic mode: show the raw HTTP exchange before decoding.
	if *raw {
		if err := rawDiagnostics(ctx, wd, *limit); err != nil {
			fmt.Fprintf(os.Stderr, "raw diagnostics failed: %v\n", err)
		}
		fmt.Println()
	}

	provider, err := workday.NewProvider(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build Workday provider: %v\n", err)
		os.Exit(2)
	}

	// 1) Fetch one page of workers.
	fmt.Println("== workers ==")
	workerPage, err := provider.FetchWorkers(ctx, "", *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch workers failed: %v\n", err)
		os.Exit(1)
	}
	printWorkers(workerPage.Items)

	// 2) Optionally fetch one page of org units.
	if *orgs {
		fmt.Println("== org units ==")
		orgPage, err := provider.FetchOrgUnits(ctx, "", *limit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fetch org units failed: %v\n", err)
			os.Exit(1)
		}
		printOrgUnits(orgPage.Items)
	}

	fmt.Println()
	fmt.Println("connectivity OK: token acquisition + data pull succeeded")
	fmt.Println("next: set WORKDAY_SYNC_ORG_UNITS per your needs and trigger")
	fmt.Println("      POST /api/v1/system/admin/integrations/workday/sync")
}

func printConfigSummary(wd *config.WorkdayConfig) {
	fmt.Println("== Workday config ==")
	fmt.Printf("  provider       : %s\n", wd.Provider)
	fmt.Printf("  connection_key : %s\n", wd.ConnectionKey)
	fmt.Printf("  base_url       : %s\n", wd.BaseURL)
	fmt.Printf("  workers_path   : %s\n", wd.WorkersPath)
	fmt.Printf("  org_units_path : %s\n", wd.OrgUnitsPath)
	fmt.Printf("  token_url      : %s\n", wd.TokenURL)
	fmt.Printf("  client_id      : %s\n", maskSecret(wd.ClientID, 6))
	if wd.ClientSecret != "" {
		fmt.Printf("  client_secret  : %s\n", maskSecret(wd.ClientSecret, 4))
	} else {
		fmt.Println("  client_secret  : <empty>")
	}
	fmt.Printf("  scope          : %q\n", wd.Scope)
	pagination := wd.Pagination
	if pagination == "" {
		pagination = "cursor"
	}
	fmt.Printf("  pagination     : %s\n", pagination)
	fmt.Printf("  sync_org_units : %v\n", wd.SyncOrgUnitsEnabled())
	fmt.Println()
}

// maskSecret keeps the first n characters and the last 2, hiding the rest.
func maskSecret(s string, keep int) string {
	if s == "" {
		return "<empty>"
	}
	if len(s) <= keep+2 {
		return s[:1] + "***"
	}
	return s[:keep] + "..." + s[len(s)-2:]
}

func printWorkers(items []types.WorkdayWorkerRecord) {
	for i, it := range items {
		fmt.Printf("  [%d] external_id=%s corporate_email=%s status=%s primary_org=%s\n",
			i+1, it.ExternalID, it.CorporateEmail, it.Status, it.PrimaryOrgExternalID)
	}
	if len(items) == 0 {
		fmt.Println("  <no workers returned>")
	}
}

func printOrgUnits(items []types.WorkdayOrgUnitRecord) {
	for i, it := range items {
		fmt.Printf("  [%d] external_id=%s code=%s name=%s org_type=%s status=%s parent=%s\n",
			i+1, it.ExternalID, it.Code, it.Name, it.OrgType, it.Status, it.ParentExternalID)
	}
	if len(items) == 0 {
		fmt.Println("  <no org units returned>")
	}
}

// rawDiagnostics performs the token + data exchange by hand and prints the
// HTTP status and a truncated response body, which is far more actionable
// for 401/403/404 debugging than the provider's wrapped error.
func rawDiagnostics(ctx context.Context, wd *config.WorkdayConfig, limit int) error {
	clientCfg := secutils.DefaultSSRFSafeHTTPClientConfig()
	clientCfg.Timeout = 60 * time.Second
	client := secutils.NewSSRFSafeHTTPClient(clientCfg)

	if wd.TokenURL != "" {
		if err := secutils.ValidateURLForSSRF(wd.TokenURL); err != nil {
			fmt.Fprintf(os.Stderr, "  token URL failed SSRF validation: %v\n", err)
			return nil
		}
		form := url.Values{}
		form.Set("grant_type", "client_credentials")
		if wd.Scope != "" {
			form.Set("scope", wd.Scope)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, wd.TokenURL, bytes.NewBufferString(form.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		req.SetBasicAuth(wd.ClientID, wd.ClientSecret)

		fmt.Println("== raw: token endpoint ==")
		fmt.Printf("  POST %s (client_id=%s)\n", wd.TokenURL, maskSecret(wd.ClientID, 6))
		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  request failed: %v\n", err)
			return nil
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		fmt.Printf("  status: %d\n", resp.StatusCode)
		fmt.Printf("  body  : %s\n", truncate(string(body), 2048))
	}

	// Data endpoint.
	parsed, err := url.Parse(wd.BaseURL + wd.WorkersPath)
	if err != nil {
		return fmt.Errorf("parse workers URL: %w", err)
	}
	q := parsed.Query()
	q.Set("page_size", fmt.Sprint(limit))
	parsed.RawQuery = q.Encode()
	if err := secutils.ValidateURLForSSRF(parsed.String()); err != nil {
		fmt.Fprintf(os.Stderr, "  workers URL failed SSRF validation: %v\n", err)
		return nil
	}

	fmt.Println("== raw: data endpoint ==")
	fmt.Printf("  GET %s\n", parsed.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	// Try to obtain a fresh token for the data call.
	if wd.TokenURL != "" {
		token := requestToken(ctx, client, wd)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("data endpoint request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	fmt.Printf("  status: %d\n", resp.StatusCode)
	fmt.Printf("  body  : %s\n", truncate(string(body), 4096))
	return nil
}

// requestToken performs a client_credentials exchange and returns the access
// token (empty on failure; the caller already saw the raw response above).
func requestToken(ctx context.Context, client *http.Client, wd *config.WorkdayConfig) string {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	if wd.Scope != "" {
		form.Set("scope", wd.Scope)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wd.TokenURL, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(wd.ClientID, wd.ClientSecret)
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode >= 300 {
		return ""
	}
	defer resp.Body.Close()
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&token); err != nil {
		return ""
	}
	return token.AccessToken
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

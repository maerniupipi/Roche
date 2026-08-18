package workday

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/types"
	secutils "roche.local/knowledge-agent-platform/internal/utils"
)

func TestMockProviderPaginatesOrganizationsAndWorkers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workday.json")
	fixture := `{
	  "org_units": [
	    {"external_id":"org-a","code":"A","name":"A","status":"active"},
	    {"external_id":"org-b","code":"B","name":"B","status":"active"},
	    {"external_id":"org-c","code":"C","name":"C","status":"active"}
	  ],
	  "workers": [
	    {"external_id":"w-1","corporate_email":"one@example.com","status":"active"},
	    {"external_id":"w-2","corporate_email":"two@example.com","status":"active"}
	  ]
	}`
	if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	provider, err := newMockProvider(path)
	if err != nil {
		t.Fatal(err)
	}

	first, err := provider.FetchOrgUnits(context.Background(), "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.NextCursor != "2" {
		t.Fatalf("first organization page = %+v", first)
	}
	second, err := provider.FetchOrgUnits(context.Background(), first.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.NextCursor != "" {
		t.Fatalf("second organization page = %+v", second)
	}

	workers, err := provider.FetchWorkers(context.Background(), "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(workers.Items) != 2 || workers.NextCursor != "" {
		t.Fatalf("worker page = %+v", workers)
	}
}

func TestMockProviderExpandsGeneratedWorkers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workday.json")
	fixture := `{
	  "org_units": [
	    {"external_id":"org-a","code":"A","name":"A","status":"active"},
	    {"external_id":"org-b","code":"B","name":"B","status":"active"}
	  ],
	  "workers": [
	    {"external_id":"w-admin","corporate_email":"admin@example.com","status":"active"}
	  ],
	  "generated_workers": {
	    "count": 100,
	    "username_prefix": "developer",
	    "email_domain": "example.com",
	    "external_id_prefix": "w-dev-",
	    "org_external_ids": ["org-a", "org-b"],
	    "manager_external_worker_id": "w-admin"
	  }
	}`
	if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	provider, err := newMockProvider(path)
	if err != nil {
		t.Fatal(err)
	}

	first, err := provider.FetchWorkers(context.Background(), "", 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 60 || first.NextCursor != "60" {
		t.Fatalf("first generated worker page = %+v", first)
	}
	second, err := provider.FetchWorkers(context.Background(), first.NextCursor, 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 41 || second.NextCursor != "" {
		t.Fatalf("second generated worker page = %+v", second)
	}
	if first.Items[1].CorporateEmail != "developer001@example.com" || first.Items[1].PrimaryOrgExternalID != "org-a" {
		t.Fatalf("first generated worker = %+v", first.Items[1])
	}
	last := second.Items[len(second.Items)-1]
	if last.CorporateEmail != "developer100@example.com" || last.PrimaryOrgExternalID != "org-b" {
		t.Fatalf("last generated worker = %+v", last)
	}
}

// allowLoopbackForTest whitelists 127.0.0.1 so the SSRF-safe client can talk
// to httptest servers. It restores the previous whitelist afterwards.
func allowLoopbackForTest(t *testing.T) {
	t.Helper()
	t.Setenv("SSRF_WHITELIST", "127.0.0.1")
	secutils.ResetSSRFWhitelistForTest()
	t.Cleanup(secutils.ResetSSRFWhitelistForTest)
}

func newOffsetTestServer(t *testing.T, total int) (*httptest.Server, *[]string) {
	t.Helper()
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RawQuery)
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		items := []map[string]any{}
		for i := offset; i < offset+limit && i <= total; i++ {
			items = append(items, map[string]any{
				"external_id":     fmt.Sprintf("w-%d", i),
				"corporate_email": fmt.Sprintf("w%d@example.com", i),
				"status":          "active",
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "total": total})
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func TestHTTPProviderOffsetPagination(t *testing.T) {
	allowLoopbackForTest(t)

	srv, requests := newOffsetTestServer(t, 5)
	cfg := &config.WorkdayConfig{
		Enable:         true,
		Provider:       "http",
		BaseURL:        srv.URL,
		WorkersPath:    "/employees",
		Pagination:     "offset",
		RequestTimeout: 10 * time.Second,
	}
	provider, err := newHTTPProvider(cfg)
	if err != nil {
		t.Fatal(err)
	}

	first, err := provider.FetchWorkers(context.Background(), "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.NextCursor != "3" {
		t.Fatalf("first page items=%d next=%q, want 2 / \"3\"", len(first.Items), first.NextCursor)
	}
	second, err := provider.FetchWorkers(context.Background(), first.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 2 || second.NextCursor != "5" {
		t.Fatalf("second page items=%d next=%q, want 2 / \"5\"", len(second.Items), second.NextCursor)
	}
	third, err := provider.FetchWorkers(context.Background(), second.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(third.Items) != 1 || third.NextCursor != "" {
		t.Fatalf("third page items=%d next=%q, want 1 / \"\"", len(third.Items), third.NextCursor)
	}
	want := []string{"limit=2&offset=1", "limit=2&offset=3", "limit=2&offset=5"}
	if !reflect.DeepEqual(*requests, want) {
		t.Fatalf("query strings = %v, want %v", *requests, want)
	}
}

func TestHTTPProviderDecodesRocheEmployees(t *testing.T) {
	allowLoopbackForTest(t)

	const fixture = `{
	  "employees": [
	    {
	      "employeeWId": "c600fde84aac01bf096cbb283621e7a6",
	      "employeeId": "61299240",
	      "persistentId": "303579",
	      "userId": "WEIF1",
	      "lastModifiedOn": "2022-11-11T09:17:23.924Z",
	      "status": { "isActive": true, "activeStatusDate": "2022-10-17" },
	      "organization": {
	        "supervisory": {
	          "id": "70058396",
	          "code": "FACE",
	          "name": "Diagnostics Informatics",
	          "manager": { "WId": "945e702ce0a71000c4dd704ef7d80000", "employeeId": "50056003" }
	        },
	        "company": { "id": "1201", "name": "F. Hoffmann-La Roche AG", "countryId": "CH" }
	      },
	      "preferredName": {
	        "prefix": "Prof. Dr.",
	        "formattedName": "Bepoh Pydyvi",
	        "reportingName": "Pydyvi, Bepoh",
	        "firstName": "Bepoh",
	        "lastName": "Pydyvi"
	      },
	      "contactInformation": {
	        "workContact": {
	          "emails": [
	            { "isPrimary": true, "emailAddress": "xcltvpwuwi@xyz.com" },
	            { "isPrimary": false, "emailAddress": "secondary@xyz.com" }
	          ]
	        }
	      }
	    },
	    {
	      "employeeWId": "d7efb1f2aabb11deadbeef0000000000",
	      "employeeId": "61299241",
	      "userId": "INAC2",
	      "status": { "isActive": false, "activeStatusDate": "2022-09-01" },
	      "organization": { "supervisory": { "id": "70000001", "code": "QAT", "name": "Quality" } },
	      "contactInformation": {
	        "workContact": { "emails": [{ "isPrimary": true, "emailAddress": "Inactive.User@xyz.com" }] }
	      }
	    }
	  ]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixture))
	}))
	t.Cleanup(srv.Close)

	cfg := &config.WorkdayConfig{
		Enable:         true,
		Provider:       "http",
		BaseURL:        srv.URL,
		WorkersPath:    "/employees",
		Pagination:     "offset",
		RequestTimeout: 10 * time.Second,
	}
	provider, err := newHTTPProvider(cfg)
	if err != nil {
		t.Fatal(err)
	}

	page, err := provider.FetchWorkers(context.Background(), "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.NextCursor != "" {
		t.Fatalf("page = %+v, want 2 items and no next cursor", page)
	}

	first := page.Items[0]
	if first.ExternalID != "c600fde84aac01bf096cbb283621e7a6" {
		t.Fatalf("first external id = %q", first.ExternalID)
	}
	if first.CorporateEmail != "xcltvpwuwi@xyz.com" {
		t.Fatalf("first corporate email = %q", first.CorporateEmail)
	}
	if first.Status != types.ExternalWorkerActive {
		t.Fatalf("first status = %q, want active", first.Status)
	}
	if first.PrimaryOrgExternalID != "70058396" {
		t.Fatalf("first primary org = %q", first.PrimaryOrgExternalID)
	}
	if first.ManagerExternalWorkerID != "945e702ce0a71000c4dd704ef7d80000" {
		t.Fatalf("first manager = %q", first.ManagerExternalWorkerID)
	}
	if got := first.Attributes["display_name"]; got != "Bepoh Pydyvi" {
		t.Fatalf("first display_name = %v", got)
	}
	if got := first.Attributes["user_id"]; got != "WEIF1" {
		t.Fatalf("first user_id = %v", got)
	}
	if got := first.Attributes["employee_id"]; got != "61299240" {
		t.Fatalf("first employee_id = %v", got)
	}
	if got := first.Attributes["supervisory_code"]; got != "FACE" {
		t.Fatalf("first supervisory_code = %v", got)
	}

	second := page.Items[1]
	if second.Status != types.ExternalWorkerInactive {
		t.Fatalf("second status = %q, want inactive", second.Status)
	}
	if second.CorporateEmail != "Inactive.User@xyz.com" {
		t.Fatalf("second email = %q", second.CorporateEmail)
	}
}

func TestHTTPProviderOffsetPaginationBareArray(t *testing.T) {
	allowLoopbackForTest(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		items := []map[string]any{}
		for i := offset; i < offset+limit && i <= 3; i++ {
			items = append(items, map[string]any{
				"external_id":     fmt.Sprintf("w-%d", i),
				"corporate_email": fmt.Sprintf("w%d@example.com", i),
				"status":          "active",
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(items)
	}))
	t.Cleanup(srv.Close)

	cfg := &config.WorkdayConfig{
		Enable:         true,
		Provider:       "http",
		BaseURL:        srv.URL,
		WorkersPath:    "/employees",
		Pagination:     "offset",
		RequestTimeout: 10 * time.Second,
	}
	provider, err := newHTTPProvider(cfg)
	if err != nil {
		t.Fatal(err)
	}

	first, err := provider.FetchWorkers(context.Background(), "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.NextCursor != "3" {
		t.Fatalf("first page items=%d next=%q, want 2 / \"3\"", len(first.Items), first.NextCursor)
	}
	last, err := provider.FetchWorkers(context.Background(), first.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(last.Items) != 1 || last.NextCursor != "" {
		t.Fatalf("last page items=%d next=%q, want 1 / \"\"", len(last.Items), last.NextCursor)
	}
}

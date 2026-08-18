package googledrive

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"roche.local/knowledge-agent-platform/internal/types"
)

type fakeDriveAPI struct {
	pingErr      error
	roots        []driveFile
	files        map[string]driveFile
	children     map[string][]driveFile
	downloads    map[string][]byte
	downloadErrs map[string]error
}

func (f *fakeDriveAPI) Ping(context.Context) error {
	return f.pingErr
}

func (f *fakeDriveAPI) ListRoots(context.Context) ([]driveFile, error) {
	return append([]driveFile(nil), f.roots...), nil
}

func (f *fakeDriveAPI) ListChildren(_ context.Context, parentID string) ([]driveFile, error) {
	return append([]driveFile(nil), f.children[parentID]...), nil
}

func (f *fakeDriveAPI) GetFile(_ context.Context, fileID string) (driveFile, error) {
	file, ok := f.files[fileID]
	if !ok {
		return driveFile{}, fmt.Errorf("file %s not found", fileID)
	}
	return file, nil
}

func (f *fakeDriveAPI) Download(
	_ context.Context,
	file driveFile,
	_ int64,
) ([]byte, string, string, error) {
	if err := f.downloadErrs[file.ID]; err != nil {
		return nil, "", "", err
	}
	fileName, contentType, err := fileNameForDownload(file)
	if err != nil {
		return nil, "", "", err
	}
	return append([]byte(nil), f.downloads[file.ID]...), contentType, fileName, nil
}

func testConfig(resourceIDs ...string) *types.DataSourceConfig {
	return &types.DataSourceConfig{
		Type: types.ConnectorTypeGoogleDrive,
		Credentials: map[string]interface{}{
			"service_account_json": `{"type":"service_account"}`,
		},
		ResourceIDs: resourceIDs,
		Settings:    map[string]interface{}{},
	}
}

func TestParseConfig(t *testing.T) {
	t.Run("missing credentials", func(t *testing.T) {
		_, err := parseConfig(&types.DataSourceConfig{})
		if err == nil {
			t.Fatal("parseConfig() expected an error")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := parseConfig(&types.DataSourceConfig{
			Credentials: map[string]interface{}{
				"service_account_json": "{",
			},
		})
		if err == nil {
			t.Fatal("parseConfig() expected an error")
		}
	})

	t.Run("delegation and size", func(t *testing.T) {
		config := testConfig()
		config.Credentials["delegated_user"] = " user@example.com "
		config.Settings["max_file_size_mb"] = float64(25)
		parsed, err := parseConfig(config)
		if err != nil {
			t.Fatalf("parseConfig() error = %v", err)
		}
		if parsed.DelegatedUser != "user@example.com" {
			t.Fatalf("DelegatedUser = %q", parsed.DelegatedUser)
		}
		if parsed.MaxFileSizeBytes != 25*1024*1024 {
			t.Fatalf("MaxFileSizeBytes = %d", parsed.MaxFileSizeBytes)
		}
	})
}

func TestFileNameForDownloadPreservesDottedNames(t *testing.T) {
	fileName, _, err := fileNameForDownload(driveFile{
		ID:       "doc",
		Name:     "Policy.v2",
		MimeType: mimeGoogleDoc,
	})
	if err != nil {
		t.Fatalf("fileNameForDownload() error = %v", err)
	}
	if fileName != "Policy.v2.docx" {
		t.Fatalf("fileName = %q, want Policy.v2.docx", fileName)
	}
}

func TestConnectorValidate(t *testing.T) {
	expected := errors.New("denied")
	connector := newConnectorWithClient(&fakeDriveAPI{pingErr: expected})
	if err := connector.Validate(context.Background(), testConfig()); !errors.Is(err, expected) {
		t.Fatalf("Validate() error = %v, want %v", err, expected)
	}
}

func TestConnectorListResources(t *testing.T) {
	client := &fakeDriveAPI{
		roots: []driveFile{
			{ID: "root", Name: "My Drive", MimeType: mimeFolder, ResourceType: "drive"},
		},
		children: map[string][]driveFile{
			"root": {
				{ID: "doc", Name: "Policy", MimeType: mimeGoogleDoc},
				{ID: "folder", Name: "Department", MimeType: mimeFolder},
			},
		},
	}
	connector := newConnectorWithClient(client)

	roots, err := connector.ListResources(context.Background(), testConfig(), "")
	if err != nil {
		t.Fatalf("ListResources(root) error = %v", err)
	}
	if len(roots) != 1 || !roots[0].HasChildren || roots[0].Type != "drive" {
		t.Fatalf("unexpected roots: %#v", roots)
	}

	children, err := connector.ListResources(context.Background(), testConfig(), "root")
	if err != nil {
		t.Fatalf("ListResources(children) error = %v", err)
	}
	if len(children) != 2 {
		t.Fatalf("len(children) = %d, want 2", len(children))
	}
	if children[0].ExternalID != "folder" || !children[0].HasChildren {
		t.Fatalf("folders must sort first: %#v", children)
	}
}

func TestConnectorFetchAllRecursivelyExportsAndDownloads(t *testing.T) {
	now := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	client := &fakeDriveAPI{
		files: map[string]driveFile{
			"root": {ID: "root", Name: "Root", MimeType: mimeFolder},
			"sub":  {ID: "sub", Name: "Sub", MimeType: mimeFolder},
			"doc":  {ID: "doc", Name: "Policy", MimeType: mimeGoogleDoc, ModifiedAt: now},
			"pdf":  {ID: "pdf", Name: "Manual.pdf", MimeType: mimePDF, ModifiedAt: now},
			"html": {ID: "html", Name: "Page.html", MimeType: "text/html", ModifiedAt: now},
		},
		children: map[string][]driveFile{
			"root": {
				{ID: "sub", Name: "Sub", MimeType: mimeFolder},
				{ID: "pdf", Name: "Manual.pdf", MimeType: mimePDF},
			},
			"sub": {
				{ID: "doc", Name: "Policy", MimeType: mimeGoogleDoc},
				{ID: "html", Name: "Page.html", MimeType: "text/html"},
			},
		},
		downloads: map[string][]byte{
			"doc": []byte("docx"),
			"pdf": []byte("pdf"),
		},
		downloadErrs: map[string]error{},
	}
	connector := newConnectorWithClient(client)
	items, err := connector.FetchAll(context.Background(), testConfig("root"), []string{"root"})
	if err != nil {
		t.Fatalf("FetchAll() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2: %#v", len(items), items)
	}

	byID := make(map[string]types.FetchedItem)
	for _, item := range items {
		byID[item.ExternalID] = item
	}
	if byID["doc"].FileName != "Policy.docx" || byID["doc"].ContentType != mimeDOCX {
		t.Fatalf("Google Doc export = %#v", byID["doc"])
	}
	if byID["pdf"].FileName != "Manual.pdf" || string(byID["pdf"].Content) != "pdf" {
		t.Fatalf("PDF download = %#v", byID["pdf"])
	}
}

func TestConnectorFetchIncrementalChangedNewAndDeleted(t *testing.T) {
	firstTime := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	client := &fakeDriveAPI{
		files: map[string]driveFile{
			"root": {ID: "root", Name: "Root", MimeType: mimeFolder},
			"one":  {ID: "one", Name: "One.pdf", MimeType: mimePDF, ModifiedAt: firstTime},
			"two":  {ID: "two", Name: "Two.pdf", MimeType: mimePDF, ModifiedAt: firstTime},
		},
		children: map[string][]driveFile{
			"root": {
				{ID: "one", Name: "One.pdf", MimeType: mimePDF, ModifiedAt: firstTime},
				{ID: "two", Name: "Two.pdf", MimeType: mimePDF, ModifiedAt: firstTime},
			},
		},
		downloads: map[string][]byte{
			"one": []byte("one"),
			"two": []byte("two"),
		},
		downloadErrs: map[string]error{},
	}
	connector := newConnectorWithClient(client)
	config := testConfig("root")

	firstItems, cursor, err := connector.FetchIncremental(context.Background(), config, nil)
	if err != nil {
		t.Fatalf("first FetchIncremental() error = %v", err)
	}
	if len(firstItems) != 2 {
		t.Fatalf("len(firstItems) = %d, want 2", len(firstItems))
	}

	secondTime := firstTime.Add(time.Hour)
	client.files["one"] = driveFile{
		ID: "one", Name: "One.pdf", MimeType: mimePDF, ModifiedAt: secondTime,
	}
	client.files["three"] = driveFile{
		ID: "three", Name: "Three.pdf", MimeType: mimePDF, ModifiedAt: secondTime,
	}
	client.children["root"] = []driveFile{
		{ID: "one", Name: "One.pdf", MimeType: mimePDF, ModifiedAt: secondTime},
		{ID: "three", Name: "Three.pdf", MimeType: mimePDF, ModifiedAt: secondTime},
	}
	client.downloads["one"] = []byte("one changed")
	client.downloads["three"] = []byte("three")

	items, _, err := connector.FetchIncremental(context.Background(), config, cursor)
	if err != nil {
		t.Fatalf("second FetchIncremental() error = %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3: %#v", len(items), items)
	}
	states := make(map[string]bool)
	for _, item := range items {
		states[item.ExternalID] = item.IsDeleted
	}
	if states["one"] || states["three"] || !states["two"] {
		t.Fatalf("unexpected change states: %#v", states)
	}
}

func TestConnectorDownloadFailureProducesRetryableErrorItem(t *testing.T) {
	now := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	client := &fakeDriveAPI{
		files: map[string]driveFile{
			"file": {
				ID:          "file",
				Name:        "Policy.pdf",
				MimeType:    mimePDF,
				ModifiedAt:  now,
				WebViewLink: "https://drive.google.com/file",
			},
		},
		children:  map[string][]driveFile{},
		downloads: map[string][]byte{},
		downloadErrs: map[string]error{
			"file": errors.New("temporary download error"),
		},
	}
	connector := newConnectorWithClient(client)
	items, cursor, err := connector.FetchIncremental(
		context.Background(),
		testConfig("file"),
		nil,
	)
	if err != nil {
		t.Fatalf("FetchIncremental() error = %v", err)
	}
	if len(items) != 1 || items[0].Metadata["error"] == "" {
		t.Fatalf("unexpected error items: %#v", items)
	}
	if items[0].URL != "" {
		t.Fatalf("error item URL = %q, want empty", items[0].URL)
	}

	signatures, ok := cursor.ConnectorCursor["file_signatures"].(map[string]interface{})
	if !ok {
		t.Fatalf("cursor signatures = %#v", cursor.ConnectorCursor["file_signatures"])
	}
	if _, exists := signatures["file"]; exists {
		t.Fatal("failed new file must not enter cursor")
	}
}

// Package googledrive implements the Google Drive data source connector.
//
// Authentication uses a Google Cloud service account. For Google Workspace,
// an optional delegated user can be configured when domain-wide delegation is
// enabled by the Workspace administrator.
package googledrive

import (
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"roche.local/knowledge-agent-platform/internal/datasource"
	"roche.local/knowledge-agent-platform/internal/types"
)

const (
	defaultMaxFileSizeBytes int64 = 50 * 1024 * 1024
	maxMaxFileSizeBytes     int64 = 200 * 1024 * 1024

	sharedWithMeResourceID = "google-drive:shared-with-me"

	mimeFolder        = "application/vnd.google-apps.folder"
	mimeShortcut      = "application/vnd.google-apps.shortcut"
	mimeGoogleDoc     = "application/vnd.google-apps.document"
	mimeGoogleSheet   = "application/vnd.google-apps.spreadsheet"
	mimeGoogleSlides  = "application/vnd.google-apps.presentation"
	mimeGoogleDrawing = "application/vnd.google-apps.drawing"

	mimeDOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	mimeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	mimePPTX = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	mimePDF  = "application/pdf"
)

// Config is the Google Drive connector configuration.
type Config struct {
	ServiceAccountJSON string
	DelegatedUser      string
	MaxFileSizeBytes   int64
}

type driveFile struct {
	ID                 string
	Name               string
	MimeType           string
	Parents            []string
	ModifiedAt         time.Time
	WebViewLink        string
	DriveID            string
	Size               int64
	MD5Checksum        string
	ShortcutTargetID   string
	ShortcutTargetMime string
	ResourceType       string
}

func (f driveFile) isFolder() bool {
	return f.MimeType == mimeFolder
}

func (f driveFile) signature() string {
	return strings.Join([]string{
		f.ModifiedAt.UTC().Format(time.RFC3339Nano),
		f.MD5Checksum,
		strconv.FormatInt(f.Size, 10),
		f.MimeType,
	}, "|")
}

type googleDriveCursor struct {
	FileSignatures map[string]string `json:"file_signatures"`
}

type exportFormat struct {
	MimeType  string
	Extension string
}

var googleWorkspaceExportFormats = map[string]exportFormat{
	mimeGoogleDoc:     {MimeType: mimeDOCX, Extension: ".docx"},
	mimeGoogleSheet:   {MimeType: mimeXLSX, Extension: ".xlsx"},
	mimeGoogleSlides:  {MimeType: mimePPTX, Extension: ".pptx"},
	mimeGoogleDrawing: {MimeType: mimePDF, Extension: ".pdf"},
}

var supportedExtensions = map[string]struct{}{
	".pdf": {}, ".txt": {}, ".docx": {}, ".doc": {}, ".epub": {},
	".mhtml": {}, ".md": {}, ".markdown": {}, ".png": {}, ".jpg": {},
	".jpeg": {}, ".gif": {}, ".csv": {}, ".xlsx": {}, ".xls": {},
	".pptx": {}, ".ppt": {}, ".json": {}, ".mp3": {}, ".wav": {},
	".m4a": {}, ".flac": {}, ".ogg": {},
}

var extensionByMIME = map[string]string{
	"text/plain":                    ".txt",
	"text/markdown":                 ".md",
	"text/csv":                      ".csv",
	"application/json":              ".json",
	mimePDF:                         ".pdf",
	mimeDOCX:                        ".docx",
	"application/msword":            ".doc",
	mimeXLSX:                        ".xlsx",
	"application/vnd.ms-excel":      ".xls",
	mimePPTX:                        ".pptx",
	"application/vnd.ms-powerpoint": ".ppt",
	"application/epub+zip":          ".epub",
	"image/png":                     ".png",
	"image/jpeg":                    ".jpg",
	"image/gif":                     ".gif",
	"audio/mpeg":                    ".mp3",
	"audio/wav":                     ".wav",
	"audio/x-wav":                   ".wav",
	"audio/mp4":                     ".m4a",
	"audio/flac":                    ".flac",
	"audio/ogg":                     ".ogg",
}

func parseConfig(config *types.DataSourceConfig) (*Config, error) {
	if config == nil {
		return nil, datasource.ErrInvalidConfig
	}

	raw, ok := config.Credentials["service_account_json"]
	if !ok {
		return nil, fmt.Errorf("%w: missing service_account_json", datasource.ErrInvalidCredentials)
	}
	serviceAccountJSON, ok := raw.(string)
	if !ok || strings.TrimSpace(serviceAccountJSON) == "" {
		return nil, fmt.Errorf("%w: service_account_json must be a non-empty string", datasource.ErrInvalidCredentials)
	}
	if !json.Valid([]byte(serviceAccountJSON)) {
		return nil, fmt.Errorf("%w: service_account_json is not valid JSON", datasource.ErrInvalidCredentials)
	}

	delegatedUser := ""
	if value, exists := config.Credentials["delegated_user"]; exists {
		var valid bool
		delegatedUser, valid = value.(string)
		if !valid {
			return nil, fmt.Errorf("%w: delegated_user must be a string", datasource.ErrInvalidCredentials)
		}
		delegatedUser = strings.TrimSpace(delegatedUser)
	}

	maxBytes := defaultMaxFileSizeBytes
	if config.Settings != nil {
		if value, exists := config.Settings["max_file_size_mb"]; exists {
			mb, err := numericInt64(value)
			maxMB := maxMaxFileSizeBytes / (1024 * 1024)
			if err != nil || mb <= 0 {
				return nil, fmt.Errorf("%w: max_file_size_mb must be a positive integer", datasource.ErrInvalidConfig)
			}
			if mb > maxMB {
				return nil, fmt.Errorf(
					"%w: max_file_size_mb cannot exceed %d",
					datasource.ErrInvalidConfig,
					maxMB,
				)
			}
			maxBytes = mb * 1024 * 1024
		}
	}

	return &Config{
		ServiceAccountJSON: serviceAccountJSON,
		DelegatedUser:      delegatedUser,
		MaxFileSizeBytes:   maxBytes,
	}, nil
}

func numericInt64(value interface{}) (int64, error) {
	switch typed := value.(type) {
	case int:
		return int64(typed), nil
	case int32:
		return int64(typed), nil
	case int64:
		return typed, nil
	case float64:
		if typed != float64(int64(typed)) {
			return 0, fmt.Errorf("not an integer")
		}
		return int64(typed), nil
	case json.Number:
		return typed.Int64()
	case string:
		return strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
	default:
		return 0, fmt.Errorf("unsupported numeric type %T", value)
	}
}

func safeFileName(name, fallbackID string) string {
	name = strings.TrimSpace(name)
	name = strings.NewReplacer("/", "_", "\\", "_", "\r", " ", "\n", " ").Replace(name)
	name = path.Base(name)
	if name == "" || name == "." {
		name = "google-drive-" + fallbackID
	}
	return name
}

func fileNameForDownload(file driveFile) (string, string, error) {
	name := safeFileName(file.Name, file.ID)
	if export, ok := googleWorkspaceExportFormats[file.MimeType]; ok {
		ext := strings.ToLower(path.Ext(name))
		if ext != export.Extension {
			name += export.Extension
		}
		return name, export.MimeType, nil
	}

	ext := strings.ToLower(path.Ext(name))
	if _, ok := supportedExtensions[ext]; ok {
		return name, file.MimeType, nil
	}
	if inferred, ok := extensionByMIME[file.MimeType]; ok {
		if ext == "" {
			name += inferred
		} else {
			name += inferred
		}
		return name, file.MimeType, nil
	}
	return "", "", fmt.Errorf("unsupported Google Drive file type %q (%s)", file.Name, file.MimeType)
}

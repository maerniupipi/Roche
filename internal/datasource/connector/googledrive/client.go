package googledrive

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const driveFileFields = "id,name,mimeType,parents,modifiedTime,webViewLink,driveId,size,md5Checksum,shortcutDetails"

type driveAPI interface {
	Ping(ctx context.Context) error
	ListRoots(ctx context.Context) ([]driveFile, error)
	ListChildren(ctx context.Context, parentID string) ([]driveFile, error)
	GetFile(ctx context.Context, fileID string) (driveFile, error)
	Download(ctx context.Context, file driveFile, maxBytes int64) ([]byte, string, string, error)
}

type googleDriveClient struct {
	service *drive.Service
}

func newGoogleDriveClient(ctx context.Context, cfg *Config) (driveAPI, error) {
	jwtConfig, err := google.JWTConfigFromJSON(
		[]byte(cfg.ServiceAccountJSON),
		drive.DriveReadonlyScope,
	)
	if err != nil {
		return nil, fmt.Errorf("parse Google service account credentials: %w", err)
	}
	if cfg.DelegatedUser != "" {
		jwtConfig.Subject = cfg.DelegatedUser
	}

	service, err := drive.NewService(
		ctx,
		option.WithHTTPClient(jwtConfig.Client(ctx)),
	)
	if err != nil {
		return nil, fmt.Errorf("create Google Drive client: %w", err)
	}
	return &googleDriveClient{service: service}, nil
}

func (c *googleDriveClient) Ping(ctx context.Context) error {
	_, err := c.service.Files.List().
		PageSize(1).
		Spaces("drive").
		Q("trashed = false").
		Fields("files(id)").
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("connect to Google Drive: %w", err)
	}
	return nil
}

func (c *googleDriveClient) ListRoots(ctx context.Context) ([]driveFile, error) {
	root, err := c.service.Files.Get("root").
		SupportsAllDrives(true).
		Fields(googleapi.Field(driveFileFields)).
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("get My Drive root: %w", err)
	}

	roots := []driveFile{
		fromGoogleFile(root),
		{
			ID:           sharedWithMeResourceID,
			Name:         "Shared with me",
			MimeType:     mimeFolder,
			ResourceType: "shared_with_me",
		},
	}
	roots[0].Name = "My Drive"
	roots[0].ResourceType = "drive"

	pageToken := ""
	for {
		call := c.service.Drives.List().
			PageSize(100).
			Fields("nextPageToken,drives(id,name)").
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		response, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("list shared drives: %w", err)
		}
		for _, sharedDrive := range response.Drives {
			roots = append(roots, driveFile{
				ID:           sharedDrive.Id,
				Name:         sharedDrive.Name,
				MimeType:     mimeFolder,
				DriveID:      sharedDrive.Id,
				ResourceType: "shared_drive",
			})
		}
		if response.NextPageToken == "" {
			break
		}
		pageToken = response.NextPageToken
	}
	return roots, nil
}

func (c *googleDriveClient) ListChildren(ctx context.Context, parentID string) ([]driveFile, error) {
	query := "trashed = false"
	if parentID == sharedWithMeResourceID {
		query += " and sharedWithMe = true"
	} else {
		query += fmt.Sprintf(" and '%s' in parents", escapeDriveQueryValue(parentID))
	}

	var files []driveFile
	pageToken := ""
	for {
		call := c.service.Files.List().
			PageSize(1000).
			Spaces("drive").
			Q(query).
			IncludeItemsFromAllDrives(true).
			SupportsAllDrives(true).
			Fields(googleapi.Field("nextPageToken,files(" + driveFileFields + ")")).
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		response, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("list Google Drive children for %s: %w", parentID, err)
		}
		for _, file := range response.Files {
			files = append(files, fromGoogleFile(file))
		}
		if response.NextPageToken == "" {
			break
		}
		pageToken = response.NextPageToken
	}
	return files, nil
}

func (c *googleDriveClient) GetFile(ctx context.Context, fileID string) (driveFile, error) {
	if fileID == sharedWithMeResourceID {
		return driveFile{
			ID:           sharedWithMeResourceID,
			Name:         "Shared with me",
			MimeType:     mimeFolder,
			ResourceType: "shared_with_me",
		}, nil
	}
	file, err := c.service.Files.Get(fileID).
		SupportsAllDrives(true).
		Fields(googleapi.Field(driveFileFields)).
		Context(ctx).
		Do()
	if err != nil {
		return driveFile{}, fmt.Errorf("get Google Drive file %s: %w", fileID, err)
	}
	return fromGoogleFile(file), nil
}

func (c *googleDriveClient) Download(
	ctx context.Context,
	file driveFile,
	maxBytes int64,
) ([]byte, string, string, error) {
	fileName, contentType, err := fileNameForDownload(file)
	if err != nil {
		return nil, "", "", err
	}
	if file.Size > maxBytes && file.Size > 0 {
		return nil, "", "", fmt.Errorf(
			"Google Drive file %q is %d bytes, exceeding the %d byte limit",
			file.Name,
			file.Size,
			maxBytes,
		)
	}

	var response *http.Response
	if export, ok := googleWorkspaceExportFormats[file.MimeType]; ok {
		response, err = c.service.Files.Export(file.ID, export.MimeType).
			Context(ctx).
			Download()
	} else {
		response, err = c.service.Files.Get(file.ID).
			SupportsAllDrives(true).
			Context(ctx).
			Download()
	}
	if err != nil {
		return nil, "", "", fmt.Errorf("download Google Drive file %q: %w", file.Name, err)
	}
	defer response.Body.Close()

	if response.ContentLength > maxBytes {
		return nil, "", "", fmt.Errorf(
			"Google Drive file %q is %d bytes, exceeding the %d byte limit",
			file.Name,
			response.ContentLength,
			maxBytes,
		)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, "", "", fmt.Errorf("read Google Drive file %q: %w", file.Name, err)
	}
	if int64(len(content)) > maxBytes {
		return nil, "", "", fmt.Errorf(
			"Google Drive file %q exceeds the %d byte limit",
			file.Name,
			maxBytes,
		)
	}
	return content, contentType, fileName, nil
}

func fromGoogleFile(file *drive.File) driveFile {
	if file == nil {
		return driveFile{}
	}
	modifiedAt, _ := time.Parse(time.RFC3339, file.ModifiedTime)
	result := driveFile{
		ID:          file.Id,
		Name:        file.Name,
		MimeType:    file.MimeType,
		Parents:     append([]string(nil), file.Parents...),
		ModifiedAt:  modifiedAt,
		WebViewLink: file.WebViewLink,
		DriveID:     file.DriveId,
		Size:        file.Size,
		MD5Checksum: file.Md5Checksum,
	}
	if file.ShortcutDetails != nil {
		result.ShortcutTargetID = file.ShortcutDetails.TargetId
		result.ShortcutTargetMime = file.ShortcutDetails.TargetMimeType
	}
	return result
}

func escapeDriveQueryValue(value string) string {
	return strings.NewReplacer("\\", "\\\\", "'", "\\'").Replace(value)
}

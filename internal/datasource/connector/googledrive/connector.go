package googledrive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"roche.local/knowledge-agent-platform/internal/datasource"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
)

// treeLog 将树形结构日志同步写入独立文件（可选），便于人工审阅完整文件树。
// 通过环境变量 GDRIVE_TREE_LOG 指定文件路径；未配置时仅输出到应用日志。
var (
	treeLogMu    sync.Mutex
	treeLogInitd bool
	treeLogFile  *os.File
	treeLogPath  string
)

// treeLogf 输出树形日志：始终写入应用日志（带 [GoogleDrive] tree 前缀），
// 同时（若配置 GDRIVE_TREE_LOG）以纯文本追加到独立文件，不包含时间戳/颜色。
func treeLogf(ctx context.Context, format string, args ...interface{}) {
	logger.Infof(ctx, "[GoogleDrive] tree "+format, args...)
	if ensureTreeLogFile() {
		treeLogMu.Lock()
		defer treeLogMu.Unlock()
		if treeLogFile != nil {
			_, _ = io.WriteString(treeLogFile, fmt.Sprintf(format+"\n", args...))
		}
	}
}

// ensureTreeLogFile 按环境变量 GDRIVE_TREE_LOG 懒打开树形日志文件（追加模式）。
func ensureTreeLogFile() bool {
	treeLogMu.Lock()
	defer treeLogMu.Unlock()
	if !treeLogInitd {
		treeLogInitd = true
		if p := strings.TrimSpace(os.Getenv("GDRIVE_TREE_LOG")); p != "" {
			f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				logger.Warnf(context.Background(), "[GoogleDrive] open tree log file %s: %v", p, err)
				return false
			}
			treeLogFile = f
			treeLogPath = p
		}
	}
	return treeLogFile != nil
}

var _ datasource.Connector = (*Connector)(nil)

type clientFactory func(context.Context, *Config) (driveAPI, error)

// Connector implements Google Drive synchronization.
type Connector struct {
	newClient clientFactory
}

// NewConnector creates a Google Drive connector backed by the official Drive API.
func NewConnector() *Connector {
	return &Connector{newClient: newGoogleDriveClient}
}

func newConnectorWithClient(client driveAPI) *Connector {
	return &Connector{
		newClient: func(context.Context, *Config) (driveAPI, error) {
			return client, nil
		},
	}
}

// Type returns the connector type identifier.
func (c *Connector) Type() string {
	return types.ConnectorTypeGoogleDrive
}

// Validate checks the service account credentials and Drive API access.
func (c *Connector) Validate(ctx context.Context, config *types.DataSourceConfig) error {
	cfg, err := parseConfig(config)
	if err != nil {
		return err
	}
	client, err := c.newClient(ctx, cfg)
	if err != nil {
		return err
	}
	return client.Ping(ctx)
}

// ListResources returns root drives or the direct children of a selected folder.
func (c *Connector) ListResources(
	ctx context.Context,
	config *types.DataSourceConfig,
	parentID string,
) ([]types.Resource, error) {
	cfg, err := parseConfig(config)
	if err != nil {
		return nil, err
	}
	client, err := c.newClient(ctx, cfg)
	if err != nil {
		return nil, err
	}

	var files []driveFile
	if parentID == "" {
		files, err = client.ListRoots(ctx)
	} else {
		files, err = client.ListChildren(ctx, parentID)
	}
	if err != nil {
		return nil, err
	}

	resources := make([]types.Resource, 0, len(files))
	for _, file := range files {
		resourceType := file.ResourceType
		if resourceType == "" {
			resourceType = resourceTypeForFile(file)
		}
		resources = append(resources, types.Resource{
			ExternalID:  file.ID,
			Name:        file.Name,
			Type:        resourceType,
			URL:         file.WebViewLink,
			ModifiedAt:  file.ModifiedAt,
			ParentID:    parentID,
			HasChildren: file.isFolder(),
			Metadata: map[string]interface{}{
				"mime_type": file.MimeType,
				"drive_id":  file.DriveID,
			},
		})
	}
	sort.SliceStable(resources, func(i, j int) bool {
		if resources[i].HasChildren != resources[j].HasChildren {
			return resources[i].HasChildren
		}
		return strings.ToLower(resources[i].Name) < strings.ToLower(resources[j].Name)
	})
	return resources, nil
}

// ResolveResourceAncestors returns every parent required to reveal a saved selection.
func (c *Connector) ResolveResourceAncestors(
	ctx context.Context,
	config *types.DataSourceConfig,
	resourceIDs []string,
) ([]string, error) {
	cfg, err := parseConfig(config)
	if err != nil {
		return nil, err
	}
	client, err := c.newClient(ctx, cfg)
	if err != nil {
		return nil, err
	}

	ancestors := make(map[string]struct{})
	for _, resourceID := range resourceIDs {
		if resourceID == "" || resourceID == sharedWithMeResourceID {
			continue
		}
		currentID := resourceID
		visited := map[string]struct{}{resourceID: {}}
		for {
			file, getErr := client.GetFile(ctx, currentID)
			if getErr != nil {
				logger.Warnf(ctx, "[GoogleDrive] resolve ancestors for %s: %v", currentID, getErr)
				break
			}
			if len(file.Parents) == 0 {
				break
			}
			parentID := file.Parents[0]
			if parentID == "" {
				break
			}
			if _, seen := visited[parentID]; seen {
				break
			}
			visited[parentID] = struct{}{}
			ancestors[parentID] = struct{}{}
			currentID = parentID
		}
	}

	result := make([]string, 0, len(ancestors))
	for id := range ancestors {
		result = append(result, id)
	}
	sort.Strings(result)
	return result, nil
}

// FetchAll recursively downloads every supported file under the selected resources.
func (c *Connector) FetchAll(
	ctx context.Context,
	config *types.DataSourceConfig,
	resourceIDs []string,
) ([]types.FetchedItem, error) {
	items, _, err := c.walk(ctx, config, resourceIDs, nil, false)
	return items, err
}

// FetchIncremental emits only new, changed, or deleted files.
func (c *Connector) FetchIncremental(
	ctx context.Context,
	config *types.DataSourceConfig,
	cursor *types.SyncCursor,
) ([]types.FetchedItem, *types.SyncCursor, error) {
	var previous googleDriveCursor
	if cursor != nil && cursor.ConnectorCursor != nil {
		payload, err := json.Marshal(cursor.ConnectorCursor)
		if err == nil {
			_ = json.Unmarshal(payload, &previous)
		}
	}
	if previous.FileSignatures == nil {
		previous.FileSignatures = make(map[string]string)
	}

	items, current, err := c.walk(
		ctx,
		config,
		config.ResourceIDs,
		previous.FileSignatures,
		true,
	)
	if err != nil {
		return nil, nil, err
	}
	cursorMap := make(map[string]interface{})
	payload, err := json.Marshal(googleDriveCursor{FileSignatures: current})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal Google Drive cursor: %w", err)
	}
	if err := json.Unmarshal(payload, &cursorMap); err != nil {
		return nil, nil, fmt.Errorf("encode Google Drive cursor: %w", err)
	}
	return items, &types.SyncCursor{
		LastSyncTime:    time.Now().UTC(),
		ConnectorCursor: cursorMap,
	}, nil
}

func (c *Connector) walk(
	ctx context.Context,
	config *types.DataSourceConfig,
	resourceIDs []string,
	previous map[string]string,
	incremental bool,
) ([]types.FetchedItem, map[string]string, error) {
	cfg, err := parseConfig(config)
	if err != nil {
		return nil, nil, err
	}
	if len(resourceIDs) == 0 {
		return nil, nil, fmt.Errorf("no Google Drive resources selected")
	}
	client, err := c.newClient(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}

	discovered, err := discoverFiles(ctx, client, resourceIDs)
	if err != nil {
		return nil, nil, err
	}
	logger.Infof(ctx, "[GoogleDrive] scan complete: discovered %d files across %d selected resource(s)",
		len(discovered), len(resourceIDs))

	current := make(map[string]string, len(discovered))
	items := make([]types.FetchedItem, 0, len(discovered))
	for _, item := range discovered {
		signature := item.file.signature()
		current[item.file.ID] = signature
		if incremental && previous[item.file.ID] == signature {
			continue
		}

		content, contentType, fileName, downloadErr := client.Download(ctx, item.file, cfg.MaxFileSizeBytes)
		if downloadErr != nil {
			if strings.Contains(downloadErr.Error(), "unsupported Google Drive file type") {
				logger.Infof(ctx, "[GoogleDrive] skip %s: %v", item.file.ID, downloadErr)
				continue
			}
			// Preserve the old signature so a failed changed file is retried.
			if prior, exists := previous[item.file.ID]; exists {
				current[item.file.ID] = prior
			} else {
				delete(current, item.file.ID)
			}
			items = append(items, types.FetchedItem{
				ExternalID:       item.file.ID,
				Title:            item.file.Name,
				UpdatedAt:        item.file.ModifiedAt,
				SourceResourceID: item.sourceResourceID,
				Metadata: map[string]string{
					"channel":   types.ChannelGoogleDrive,
					"mime_type": item.file.MimeType,
					"error":     downloadErr.Error(),
				},
			})
			continue
		}

		items = append(items, types.FetchedItem{
			ExternalID:       item.file.ID,
			Title:            item.file.Name,
			Content:          content,
			ContentType:      contentType,
			FileName:         fileName,
			URL:              item.file.WebViewLink,
			UpdatedAt:        item.file.ModifiedAt,
			SourceResourceID: item.sourceResourceID,
			Metadata: map[string]string{
				"channel":       types.ChannelGoogleDrive,
				"mime_type":     item.file.MimeType,
				"drive_id":      item.file.DriveID,
				"md5_checksum":  item.file.MD5Checksum,
				"modified_time": item.file.ModifiedAt.UTC().Format(time.RFC3339Nano),
			},
		})
	}

	if incremental {
		for fileID := range previous {
			if _, exists := current[fileID]; !exists {
				items = append(items, types.FetchedItem{
					ExternalID: fileID,
					IsDeleted:  true,
					Metadata: map[string]string{
						"channel": types.ChannelGoogleDrive,
					},
				})
			}
		}
	}
	return items, current, nil
}

type discoveredFile struct {
	file             driveFile
	sourceResourceID string
}

func discoverFiles(
	ctx context.Context,
	client driveAPI,
	resourceIDs []string,
) ([]discoveredFile, error) {
	type pendingResource struct {
		id               string
		sourceResourceID string
		file             *driveFile
		depth            int
	}

	stack := make([]pendingResource, 0, len(resourceIDs))
	for index := len(resourceIDs) - 1; index >= 0; index-- {
		stack = append(stack, pendingResource{
			id:               resourceIDs[index],
			sourceResourceID: resourceIDs[index],
			depth:            0,
		})
	}

	visited := make(map[string]struct{})
	var files []discoveredFile
	for len(stack) > 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		if _, exists := visited[current.id]; exists {
			continue
		}
		visited[current.id] = struct{}{}

		var file driveFile
		if current.file != nil {
			file = *current.file
		} else {
			var err error
			file, err = client.GetFile(ctx, current.id)
			if err != nil {
				return nil, fmt.Errorf("inspect Google Drive resource %s: %w", current.id, err)
			}
		}
		if file.MimeType == mimeShortcut {
			if file.ShortcutTargetID == "" {
				treeLogf(ctx, "%s%s -> (shortcut, no target, skipped)", indent(current.depth), file.Name)
				continue
			}
			treeLogf(ctx, "%s%s -> %s (shortcut, id=%s)", indent(current.depth), file.Name, file.ShortcutTargetID, file.ID)
			stack = append(stack, pendingResource{
				id:               file.ShortcutTargetID,
				sourceResourceID: current.sourceResourceID,
				depth:            current.depth,
			})
			continue
		}
		if file.isFolder() {
			treeLogf(ctx, "%s%s/ (folder, id=%s)", indent(current.depth), file.Name, file.ID)
			children, listErr := client.ListChildren(ctx, file.ID)
			if listErr != nil {
				return nil, fmt.Errorf("walk Google Drive folder %s: %w", file.ID, listErr)
			}
			for index := len(children) - 1; index >= 0; index-- {
				child := children[index]
				if child.ID == "" {
					continue
				}
				stack = append(stack, pendingResource{
					id:               child.ID,
					sourceResourceID: current.sourceResourceID,
					file:             &child,
					depth:            current.depth + 1,
				})
			}
			continue
		}
		treeLogf(ctx, "%s%s (file, id=%s, mime=%s, size=%d)",
			indent(current.depth), file.Name, file.ID, file.MimeType, file.Size)
		files = append(files, discoveredFile{
			file:             file,
			sourceResourceID: current.sourceResourceID,
		})
	}
	return files, nil
}

// indent returns a two-space-per-level indentation prefix for tree logs.
func indent(depth int) string {
	return strings.Repeat("  ", depth)
}

func resourceTypeForFile(file driveFile) string {
	switch file.MimeType {
	case mimeFolder:
		return "folder"
	case mimeGoogleDoc:
		return "google_doc"
	case mimeGoogleSheet:
		return "google_sheet"
	case mimeGoogleSlides:
		return "google_slides"
	case mimeGoogleDrawing:
		return "google_drawing"
	case mimeShortcut:
		return "shortcut"
	default:
		return "file"
	}
}

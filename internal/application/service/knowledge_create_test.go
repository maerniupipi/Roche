package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type createKnowledgeFileRepoStub struct {
	interfaces.KnowledgeRepository

	createCalls      int
	createErr        error
	createdKnowledge *types.Knowledge
}

func (r *createKnowledgeFileRepoStub) CheckKnowledgeExists(
	ctx context.Context,
	knowledgeDomainID uint64,
	kbID string,
	params *types.KnowledgeCheckParams,
) (bool, *types.Knowledge, error) {
	return false, nil, nil
}

func (r *createKnowledgeFileRepoStub) ResolveKnowledgeFolderPath(
	ctx context.Context,
	knowledgeDomainID uint64,
	kbID string,
	relativePath string,
) (*types.KnowledgeFolder, error) {
	if relativePath == "" {
		return nil, nil
	}
	return &types.KnowledgeFolder{
		ID:                "folder-1",
		KnowledgeDomainID: knowledgeDomainID,
		KnowledgeBaseID:   kbID,
		Name:              "child",
		RelativePath:      relativePath,
	}, nil
}

func (r *createKnowledgeFileRepoStub) CreateKnowledge(ctx context.Context, knowledge *types.Knowledge) error {
	r.createCalls++
	copied := *knowledge
	r.createdKnowledge = &copied
	return r.createErr
}

// GetKnowledgeTags is invoked by setAndAttachKnowledgeTags after create even
// when no tags were supplied; a fresh knowledge has none, so return empty.
func (r *createKnowledgeFileRepoStub) GetKnowledgeTags(
	ctx context.Context,
	knowledgeIDs []string,
) (map[string][]*types.KnowledgeTag, error) {
	return map[string][]*types.KnowledgeTag{}, nil
}

type createKnowledgeFileKBServiceStub struct {
	interfaces.KnowledgeBaseService

	kb *types.KnowledgeBase
}

func (s *createKnowledgeFileKBServiceStub) GetKnowledgeBaseByID(
	ctx context.Context,
	id string,
) (*types.KnowledgeBase, error) {
	return s.kb, nil
}

type createKnowledgeDomainRepoStub struct {
	interfaces.KnowledgeDomainRepository

	domain *types.KnowledgeDomain
	calls  int
}

func (s *createKnowledgeDomainRepoStub) GetKnowledgeDomainByID(
	ctx context.Context,
	id uint64,
) (*types.KnowledgeDomain, error) {
	s.calls++
	if s.domain == nil {
		return &types.KnowledgeDomain{ID: id}, nil
	}
	return s.domain, nil
}

type createKnowledgeFileServiceStub struct {
	saveErr              error
	saveCalls            int
	savedWithKnowledgeID string
	deleteCalls          int
	deletedPath          string
}

func (s *createKnowledgeFileServiceStub) CheckConnectivity(ctx context.Context) error {
	return nil
}

func (s *createKnowledgeFileServiceStub) SaveFile(
	ctx context.Context,
	file *multipart.FileHeader,
	knowledgeDomainID uint64,
	knowledgeID string,
) (string, error) {
	s.saveCalls++
	s.savedWithKnowledgeID = knowledgeID
	if s.saveErr != nil {
		return "", s.saveErr
	}
	return "stored/" + knowledgeID, nil
}

func (s *createKnowledgeFileServiceStub) SaveBytes(
	ctx context.Context,
	data []byte,
	knowledgeDomainID uint64,
	fileName string,
	temp bool,
) (string, error) {
	return "", errors.New("not implemented")
}

func (s *createKnowledgeFileServiceStub) GetFile(ctx context.Context, filePath string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func (s *createKnowledgeFileServiceStub) GetFileURL(ctx context.Context, filePath string) (string, error) {
	return "", errors.New("not implemented")
}

func (s *createKnowledgeFileServiceStub) DeleteFile(ctx context.Context, filePath string) error {
	s.deleteCalls++
	s.deletedPath = filePath
	return nil
}

func (s *createKnowledgeFileServiceStub) CopyFile(ctx context.Context, srcPath string, knowledgeDomainID uint64, knowledgeID string) (string, error) {
	return "", errors.New("not implemented")
}

type createKnowledgeTaskEnqueuerStub struct {
	calls int
}

func (s *createKnowledgeTaskEnqueuerStub) Enqueue(
	task *asynq.Task,
	opts ...asynq.Option,
) (*asynq.TaskInfo, error) {
	s.calls++
	return &asynq.TaskInfo{ID: "task-1", Queue: "default"}, nil
}

func TestCreateKnowledgeFromFileDoesNotPersistWhenStorageSaveFails(t *testing.T) {
	t.Parallel()

	repo := &createKnowledgeFileRepoStub{}
	fileSvc := &createKnowledgeFileServiceStub{saveErr: errors.New("storage unavailable")}
	svc := &knowledgeService{
		repo:      repo,
		kbService: &createKnowledgeFileKBServiceStub{kb: &types.KnowledgeBase{ID: "kb-1"}},
		fileSvc:   fileSvc,
	}

	knowledge, err := svc.CreateKnowledgeFromFile(
		newCreateKnowledgeFileContext(),
		"kb-1",
		newMultipartFileHeader(t, "doc.txt", "hello"),
		nil,
		nil,
		"",
		"",
		nil,
		"",
		nil,
	)

	require.Error(t, err)
	require.Nil(t, knowledge)
	require.Equal(t, 1, fileSvc.saveCalls)
	require.Zero(t, repo.createCalls)
}

func TestCreateKnowledgeFromFilePersistsStoredFilePathOnCreate(t *testing.T) {
	t.Parallel()

	repo := &createKnowledgeFileRepoStub{}
	fileSvc := &createKnowledgeFileServiceStub{}
	task := &createKnowledgeTaskEnqueuerStub{}
	svc := &knowledgeService{
		repo:      repo,
		kbService: &createKnowledgeFileKBServiceStub{kb: &types.KnowledgeBase{ID: "kb-1"}},
		fileSvc:   fileSvc,
		task:      task,
	}

	knowledge, err := svc.CreateKnowledgeFromFile(
		newCreateKnowledgeFileContext(),
		"kb-1",
		newMultipartFileHeader(t, "doc.txt", "hello"),
		nil,
		nil,
		"",
		"",
		nil,
		"",
		nil,
	)

	require.NoError(t, err)
	require.NotNil(t, knowledge)
	require.Equal(t, 1, fileSvc.saveCalls)
	require.NotEmpty(t, fileSvc.savedWithKnowledgeID)
	require.Equal(t, fileSvc.savedWithKnowledgeID, knowledge.ID)
	require.Equal(t, 1, repo.createCalls)
	require.NotNil(t, repo.createdKnowledge)
	require.Equal(t, "stored/"+knowledge.ID, repo.createdKnowledge.FilePath)
	require.Equal(t, 1, task.calls)
}

func TestCreateKnowledgeFromFileDeletesStoredFileWhenCreateFails(t *testing.T) {
	t.Parallel()

	repo := &createKnowledgeFileRepoStub{createErr: errors.New("database unavailable")}
	fileSvc := &createKnowledgeFileServiceStub{}
	svc := &knowledgeService{
		repo:      repo,
		kbService: &createKnowledgeFileKBServiceStub{kb: &types.KnowledgeBase{ID: "kb-1"}},
		fileSvc:   fileSvc,
	}

	knowledge, err := svc.CreateKnowledgeFromFile(
		newCreateKnowledgeFileContext(),
		"kb-1",
		newMultipartFileHeader(t, "doc.txt", "hello"),
		nil,
		nil,
		"",
		"",
		nil,
		"",
		nil,
	)

	require.EqualError(t, err, "database unavailable")
	require.Nil(t, knowledge)
	require.Equal(t, 1, fileSvc.saveCalls)
	require.Equal(t, 1, repo.createCalls)
	require.Equal(t, 1, fileSvc.deleteCalls)
	require.Equal(t, "stored/"+fileSvc.savedWithKnowledgeID, fileSvc.deletedPath)
}

func TestCreateKnowledgeFromFile_PersistsProcessOverrides(t *testing.T) {
	t.Parallel()

	repo := &createKnowledgeFileRepoStub{}
	fileSvc := &createKnowledgeFileServiceStub{}
	task := &createKnowledgeTaskEnqueuerStub{}
	svc := &knowledgeService{
		repo:      repo,
		kbService: &createKnowledgeFileKBServiceStub{kb: &types.KnowledgeBase{ID: "kb-1"}},
		fileSvc:   fileSvc,
		task:      task,
	}

	chunkSize := 512
	overrides := &types.KnowledgeProcessOverrides{
		ChunkingConfig: &types.ChunkingConfig{ChunkSize: chunkSize},
	}

	knowledge, err := svc.CreateKnowledgeFromFile(
		newCreateKnowledgeFileContext(),
		"kb-1",
		newMultipartFileHeader(t, "doc.txt", "hello"),
		map[string]string{"source": "test"},
		nil,
		"",
		"",
		nil,
		"",
		overrides,
	)

	require.NoError(t, err)
	require.NotNil(t, knowledge)
	require.Equal(t, 1, repo.createCalls)
	require.NotNil(t, repo.createdKnowledge)

	parsed, err := repo.createdKnowledge.ProcessOverrides()
	require.NoError(t, err)
	require.NotNil(t, parsed)
	require.NotNil(t, parsed.ChunkingConfig)
	require.Equal(t, chunkSize, parsed.ChunkingConfig.ChunkSize)

	metadataMap, err := repo.createdKnowledge.Metadata.Map()
	require.NoError(t, err)
	require.Equal(t, "test", metadataMap["source"])
}

func TestNormalizeUploadedFilePlacement(t *testing.T) {
	t.Parallel()

	fileName, folderPath, err := normalizeUploadedFilePlacement(
		"Policies/Finance/expense.pdf",
		"",
	)
	require.NoError(t, err)
	require.Equal(t, "expense.pdf", fileName)
	require.Equal(t, "Policies/Finance", folderPath)

	fileName, folderPath, err = normalizeUploadedFilePlacement(
		"expense.pdf",
		"Policies\\Finance",
	)
	require.NoError(t, err)
	require.Equal(t, "expense.pdf", fileName)
	require.Equal(t, "Policies/Finance", folderPath)
}

func TestNormalizeKnowledgeFolderPathRejectsTraversal(t *testing.T) {
	t.Parallel()

	_, err := normalizeKnowledgeFolderPath("Policies/../Finance")
	require.Error(t, err)
}

func TestCreateKnowledgeFromFilePersistsFolderPlacement(t *testing.T) {
	t.Parallel()

	repo := &createKnowledgeFileRepoStub{}
	svc := &knowledgeService{
		repo:      repo,
		kbService: &createKnowledgeFileKBServiceStub{kb: &types.KnowledgeBase{ID: "kb-1"}},
		fileSvc:   &createKnowledgeFileServiceStub{},
		task:      &createKnowledgeTaskEnqueuerStub{},
	}

	knowledge, err := svc.CreateKnowledgeFromFile(
		newCreateKnowledgeFileContext(),
		"kb-1",
		newMultipartFileHeader(t, "expense.pdf", "hello"),
		nil,
		nil,
		"",
		"Policies/Finance",
		nil,
		"",
		nil,
	)

	require.NoError(t, err)
	require.NotNil(t, knowledge.FolderID)
	require.Equal(t, "folder-1", *knowledge.FolderID)
	require.Equal(t, "Policies/Finance", knowledge.FolderPath)
	require.Equal(t, "expense.pdf", knowledge.FileName)
}

func TestCreateKnowledgeFromFileLoadsDomainFromKnowledgeBaseWithoutDomainInfoContext(t *testing.T) {
	t.Parallel()

	repo := &createKnowledgeFileRepoStub{}
	domainRepo := &createKnowledgeDomainRepoStub{
		domain: &types.KnowledgeDomain{ID: 7, StorageQuota: 1024, StorageUsed: 0},
	}
	svc := &knowledgeService{
		repo:                repo,
		kbService:           &createKnowledgeFileKBServiceStub{kb: &types.KnowledgeBase{ID: "kb-1", KnowledgeDomainID: 7}},
		knowledgeDomainRepo: domainRepo,
		fileSvc:             &createKnowledgeFileServiceStub{},
		task:                &createKnowledgeTaskEnqueuerStub{},
	}

	knowledge, err := svc.CreateKnowledgeFromFile(
		context.Background(),
		"kb-1",
		newMultipartFileHeader(t, "doc.txt", "hello"),
		nil,
		nil,
		"",
		"PoC Data Folder - Compliance",
		nil,
		"",
		nil,
	)

	require.NoError(t, err)
	require.NotNil(t, knowledge)
	require.Equal(t, uint64(7), knowledge.KnowledgeDomainID)
	require.Equal(t, 1, domainRepo.calls)
	require.Equal(t, "PoC Data Folder - Compliance", knowledge.FolderPath)
}

func newCreateKnowledgeFileContext() context.Context {
	ctx := context.WithValue(context.Background(), types.KnowledgeDomainIDContextKey, uint64(1))
	ctx = context.WithValue(ctx, types.KnowledgeDomainInfoContextKey, &types.KnowledgeDomain{})
	return ctx
}

func newMultipartFileHeader(t *testing.T, filename string, content string) *multipart.FileHeader {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest("POST", "/", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	require.NoError(t, req.ParseMultipartForm(1024))
	return req.MultipartForm.File["file"][0]
}

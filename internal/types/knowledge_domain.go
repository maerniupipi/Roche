package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// retrieverEngineMapping maps RETRIEVE_DRIVER values to retriever engine configurations
var retrieverEngineMapping = map[string][]RetrieverEngineParams{
	"elasticsearch_v7": {
		{RetrieverType: KeywordsRetrieverType, RetrieverEngineType: ElasticsearchRetrieverEngineType},
	},
	"elasticsearch_v8": {
		{RetrieverType: KeywordsRetrieverType, RetrieverEngineType: ElasticsearchRetrieverEngineType},
		{RetrieverType: VectorRetrieverType, RetrieverEngineType: ElasticsearchRetrieverEngineType},
	},
	"qdrant": {
		{RetrieverType: KeywordsRetrieverType, RetrieverEngineType: QdrantRetrieverEngineType},
		{RetrieverType: VectorRetrieverType, RetrieverEngineType: QdrantRetrieverEngineType},
	},
	"milvus": {
		{RetrieverType: VectorRetrieverType, RetrieverEngineType: MilvusRetrieverEngineType},
		{RetrieverType: KeywordsRetrieverType, RetrieverEngineType: MilvusRetrieverEngineType},
	},
	"weaviate": {
		{RetrieverType: KeywordsRetrieverType, RetrieverEngineType: WeaviateRetrieverEngineType},
		{RetrieverType: VectorRetrieverType, RetrieverEngineType: WeaviateRetrieverEngineType},
	},
	"doris": {
		{RetrieverType: KeywordsRetrieverType, RetrieverEngineType: DorisRetrieverEngineType},
		{RetrieverType: VectorRetrieverType, RetrieverEngineType: DorisRetrieverEngineType},
	},
	"sqlite": {
		{RetrieverType: KeywordsRetrieverType, RetrieverEngineType: SQLiteRetrieverEngineType},
		{RetrieverType: VectorRetrieverType, RetrieverEngineType: SQLiteRetrieverEngineType},
	},
	"tencent_vectordb": {
		{RetrieverType: KeywordsRetrieverType, RetrieverEngineType: TencentVectorDBRetrieverEngineType},
		{RetrieverType: VectorRetrieverType, RetrieverEngineType: TencentVectorDBRetrieverEngineType},
	},
	"opensearch": {
		{RetrieverType: KeywordsRetrieverType, RetrieverEngineType: OpenSearchRetrieverEngineType},
		{RetrieverType: VectorRetrieverType, RetrieverEngineType: OpenSearchRetrieverEngineType},
	},
}

// GetRetrieverEngineMapping returns the retriever engine mapping
// This allows other packages to access the driver capabilities
func GetRetrieverEngineMapping() map[string][]RetrieverEngineParams {
	return retrieverEngineMapping
}

// GetDefaultRetrieverEngines returns the default retriever engines based on RETRIEVE_DRIVER env
func GetDefaultRetrieverEngines() []RetrieverEngineParams {
	result := []RetrieverEngineParams{}
	seen := make(map[string]bool)

	for _, driver := range strings.Split(os.Getenv("RETRIEVE_DRIVER"), ",") {
		driver = strings.TrimSpace(driver)
		if params, ok := retrieverEngineMapping[driver]; ok {
			for _, p := range params {
				key := string(p.RetrieverType) + ":" + string(p.RetrieverEngineType)
				if !seen[key] {
					seen[key] = true
					result = append(result, p)
				}
			}
		}
	}
	return result
}

// KnowledgeDomain is a knowledge-management boundary. It is independent from
// the enterprise organization tree.
//
// Identity fields are stored in knowledge_domains, storage accounting lives in
// knowledge_domain_storage, and process-wide runtime settings live in
// platform_runtime_configs. Runtime settings are hydrated for read APIs because
// knowledge-base creation consumes the platform defaults.
type KnowledgeDomain struct {
	// ID
	ID uint64 `yaml:"id"                  json:"id"                  gorm:"primaryKey"`
	// Code is the stable, human-safe identifier of the knowledgeDomain.
	Code string `yaml:"code"                json:"code"                gorm:"type:varchar(64);not null;uniqueIndex"`
	// Name
	Name string `yaml:"name"                json:"name"`
	// NameEn is the English display name, used for bilingual menu titles.
	NameEn string `yaml:"name_en"            json:"name_en"            gorm:"type:varchar(255);not null;default:''"`
	// Description
	Description string `yaml:"description"         json:"description"`
	// Status
	Status string `yaml:"status"              json:"status"              gorm:"default:'active'"`
	// Runtime and accounting fields are read-only projections. They do not
	// belong to knowledge_domains and are hydrated from the dedicated tables.
	RetrieverEngines    RetrieverEngines     `yaml:"retriever_engines" json:"retriever_engines" gorm:"-"`
	StorageQuota        int64                `yaml:"storage_quota" json:"storage_quota" gorm:"-"`
	StorageUsed         int64                `yaml:"storage_used" json:"storage_used" gorm:"-"`
	ContextConfig       *ContextConfig       `yaml:"context_config" json:"context_config" gorm:"-"`
	WebSearchConfig     *WebSearchConfig     `yaml:"web_search_config" json:"web_search_config" gorm:"-"`
	ParserEngineConfig  *ParserEngineConfig  `yaml:"parser_engine_config" json:"parser_engine_config" gorm:"-"`
	StorageEngineConfig *StorageEngineConfig `yaml:"storage_engine_config" json:"storage_engine_config" gorm:"-"`
	RetrievalConfig     *RetrievalConfig     `yaml:"retrieval_config" json:"retrieval_config" gorm:"-"`
	// Creation time
	CreatedAt time.Time `yaml:"created_at"          json:"created_at"`
	// Last updated time
	UpdatedAt time.Time `yaml:"updated_at"          json:"updated_at"`
	// Deletion time
	DeletedAt gorm.DeletedAt `yaml:"deleted_at"          json:"deleted_at"          gorm:"index"`
}

func (KnowledgeDomain) TableName() string {
	return "knowledge_domains"
}

// KnowledgeDomainStorage stores mutable accounting separately from knowledge
// domain identity. A domain has exactly one row.
type KnowledgeDomainStorage struct {
	KnowledgeDomainID uint64    `json:"knowledge_domain_id" gorm:"primaryKey;column:knowledge_domain_id"`
	StorageQuota      int64     `json:"storage_quota" gorm:"not null;default:10737418240"`
	StorageUsed       int64     `json:"storage_used" gorm:"not null;default:0"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (KnowledgeDomainStorage) TableName() string {
	return "knowledge_domain_storage"
}

// PlatformRuntimeConfig is the singleton configuration shared by every
// knowledge domain. ID is always 1.
type PlatformRuntimeConfig struct {
	ID                  uint8                `json:"id" gorm:"primaryKey;default:1"`
	RetrieverEngines    RetrieverEngines     `json:"retriever_engines" gorm:"type:jsonb;not null"`
	ContextConfig       *ContextConfig       `json:"context_config" gorm:"type:jsonb"`
	WebSearchConfig     *WebSearchConfig     `json:"web_search_config" gorm:"type:jsonb"`
	ParserEngineConfig  *ParserEngineConfig  `json:"parser_engine_config" gorm:"type:jsonb"`
	StorageEngineConfig *StorageEngineConfig `json:"storage_engine_config" gorm:"type:jsonb"`
	RetrievalConfig     *RetrievalConfig     `json:"retrieval_config" gorm:"type:jsonb"`
	CreatedAt           time.Time            `json:"created_at"`
	UpdatedAt           time.Time            `json:"updated_at"`
}

func (PlatformRuntimeConfig) TableName() string {
	return "platform_runtime_configs"
}

// RetrieverEngines represents the platform-wide retriever engine selection.
type RetrieverEngines struct {
	Engines []RetrieverEngineParams `yaml:"engines" json:"engines" gorm:"type:json"`
}

// GetEffectiveEngines returns the hydrated platform engines, falling back to
// environment defaults when the database has not been initialized yet.
func (t *KnowledgeDomain) GetEffectiveEngines() []RetrieverEngineParams {
	if len(t.RetrieverEngines.Engines) > 0 {
		return t.RetrieverEngines.Engines
	}
	return GetDefaultRetrieverEngines()
}

// BeforeCreate initializes the stable knowledge-domain code.
func (t *KnowledgeDomain) BeforeCreate(tx *gorm.DB) error {
	if strings.TrimSpace(t.Code) == "" {
		t.Code = "kd_" + uuid.NewString()
	}
	return nil
}

// Value implements the driver.Valuer interface, used to convert RetrieverEngines to database value
func (c RetrieverEngines) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements sql.Scanner for the object-wrapped database representation:
// {"engines": [{...}, {...}]}.
func (c *RetrieverEngines) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}

	if err := json.Unmarshal(b, c); err != nil {
		return fmt.Errorf("retriever_engines: cannot unmarshal object: %w", err)
	}
	return nil
}

// ParserEngineConfig holds platform-wide document parser settings.
// These values take precedence over environment variables when parsing documents.
type ParserEngineConfig struct {
	MinerUEndpoint string `json:"mineru_endpoint"` // MinerU 自建服务端点
	MinerUAPIKey   string `json:"mineru_api_key"`  // MinerU 云 API Key

	// MinerU 自建解析参数
	MinerUModel         string `json:"mineru_model,omitempty"`          // backend: pipeline, vlm-*, hybrid-*
	MinerUVLMServerURL  string `json:"mineru_vlm_server_url,omitempty"` // vLLM 服务器地址 (vlm-http-client / hybrid-http-client)
	MinerUEnableFormula *bool  `json:"mineru_enable_formula,omitempty"`
	MinerUEnableTable   *bool  `json:"mineru_enable_table,omitempty"`
	MinerUEnableOCR     *bool  `json:"mineru_enable_ocr,omitempty"`
	MinerULanguage      string `json:"mineru_language,omitempty"`

	// MinerU 云 API 解析参数
	MinerUCloudModel         string `json:"mineru_cloud_model,omitempty"` // model_version: pipeline, vlm, MinerU-HTML
	MinerUCloudEnableFormula *bool  `json:"mineru_cloud_enable_formula,omitempty"`
	MinerUCloudEnableTable   *bool  `json:"mineru_cloud_enable_table,omitempty"`
	MinerUCloudEnableOCR     *bool  `json:"mineru_cloud_enable_ocr,omitempty"`
	MinerUCloudLanguage      string `json:"mineru_cloud_language,omitempty"`

	// OpenDataLoader PDF (docreader engine); hybrid requires opendataloader-pdf-hybrid service.
	ODLHybrid           string `json:"odl_hybrid,omitempty"`      // off (default), docling-fast, hancom-ai
	ODLHybridURL        string `json:"odl_hybrid_url,omitempty"`  // e.g. http://odl-hybrid:5002
	ODLHybridMode       string `json:"odl_hybrid_mode,omitempty"` // auto, full
	ODLHybridFallback   *bool  `json:"odl_hybrid_fallback,omitempty"`
	ODLMarkdownWithHTML *bool  `json:"odl_markdown_with_html,omitempty"`

	// PaddleOCR-VL self-hosted pipeline service (full /layout-parsing API).
	PaddleOCRVLEndpoint            string `json:"paddleocr_vl_endpoint,omitempty"` // e.g. http://paddleocr-vl:8080
	PaddleOCRVLUseSealRecognition  *bool  `json:"paddleocr_vl_use_seal_recognition,omitempty"`
	PaddleOCRVLUseChartRecognition *bool  `json:"paddleocr_vl_use_chart_recognition,omitempty"`

	// PaddleOCR-VL AI Studio cloud API.
	PaddleOCRVLCloudToken               string `json:"paddleocr_vl_cloud_token,omitempty"`
	PaddleOCRVLCloudModel               string `json:"paddleocr_vl_cloud_model,omitempty"` // e.g. PaddleOCR-VL-1.6
	PaddleOCRVLCloudUseSealRecognition  *bool  `json:"paddleocr_vl_cloud_use_seal_recognition,omitempty"`
	PaddleOCRVLCloudUseChartRecognition *bool  `json:"paddleocr_vl_cloud_use_chart_recognition,omitempty"`
}

// ToOverridesMap returns a map suitable for ParserEngineOverrides in parse requests.
// Keys are snake_case (mineru_endpoint, mineru_api_key, etc.).
func (c *ParserEngineConfig) ToOverridesMap() map[string]string {
	if c == nil {
		return nil
	}
	m := make(map[string]string)
	if c.MinerUEndpoint != "" {
		m["mineru_endpoint"] = c.MinerUEndpoint
	}
	if c.MinerUAPIKey != "" {
		m["mineru_api_key"] = c.MinerUAPIKey
	}
	if c.MinerUModel != "" {
		m["mineru_model"] = c.MinerUModel
	}
	if c.MinerUVLMServerURL != "" {
		m["mineru_vlm_server_url"] = c.MinerUVLMServerURL
	}
	if c.MinerUEnableFormula != nil {
		m["mineru_enable_formula"] = fmt.Sprintf("%v", *c.MinerUEnableFormula)
	}
	if c.MinerUEnableTable != nil {
		m["mineru_enable_table"] = fmt.Sprintf("%v", *c.MinerUEnableTable)
	}
	if c.MinerUEnableOCR != nil {
		m["mineru_enable_ocr"] = fmt.Sprintf("%v", *c.MinerUEnableOCR)
	}
	if c.MinerULanguage != "" {
		m["mineru_language"] = c.MinerULanguage
	}
	if c.MinerUCloudModel != "" {
		m["mineru_cloud_model"] = c.MinerUCloudModel
	}
	if c.MinerUCloudEnableFormula != nil {
		m["mineru_cloud_enable_formula"] = fmt.Sprintf("%v", *c.MinerUCloudEnableFormula)
	}
	if c.MinerUCloudEnableTable != nil {
		m["mineru_cloud_enable_table"] = fmt.Sprintf("%v", *c.MinerUCloudEnableTable)
	}
	if c.MinerUCloudEnableOCR != nil {
		m["mineru_cloud_enable_ocr"] = fmt.Sprintf("%v", *c.MinerUCloudEnableOCR)
	}
	if c.MinerUCloudLanguage != "" {
		m["mineru_cloud_language"] = c.MinerUCloudLanguage
	}
	if c.ODLHybrid != "" {
		m["odl_hybrid"] = c.ODLHybrid
	}
	if c.ODLHybridURL != "" {
		m["odl_hybrid_url"] = c.ODLHybridURL
	}
	if c.ODLHybridMode != "" {
		m["odl_hybrid_mode"] = c.ODLHybridMode
	}
	if c.ODLHybridFallback != nil {
		m["odl_hybrid_fallback"] = fmt.Sprintf("%v", *c.ODLHybridFallback)
	}
	if c.ODLMarkdownWithHTML != nil {
		m["odl_markdown_with_html"] = fmt.Sprintf("%v", *c.ODLMarkdownWithHTML)
	}
	if c.PaddleOCRVLEndpoint != "" {
		m["paddleocr_vl_endpoint"] = c.PaddleOCRVLEndpoint
	}
	if c.PaddleOCRVLUseSealRecognition != nil {
		m["paddleocr_vl_use_seal_recognition"] = fmt.Sprintf("%v", *c.PaddleOCRVLUseSealRecognition)
	}
	if c.PaddleOCRVLUseChartRecognition != nil {
		m["paddleocr_vl_use_chart_recognition"] = fmt.Sprintf("%v", *c.PaddleOCRVLUseChartRecognition)
	}
	if c.PaddleOCRVLCloudToken != "" {
		m["paddleocr_vl_cloud_token"] = c.PaddleOCRVLCloudToken
	}
	if c.PaddleOCRVLCloudModel != "" {
		m["paddleocr_vl_cloud_model"] = c.PaddleOCRVLCloudModel
	}
	if c.PaddleOCRVLCloudUseSealRecognition != nil {
		m["paddleocr_vl_cloud_use_seal_recognition"] = fmt.Sprintf("%v", *c.PaddleOCRVLCloudUseSealRecognition)
	}
	if c.PaddleOCRVLCloudUseChartRecognition != nil {
		m["paddleocr_vl_cloud_use_chart_recognition"] = fmt.Sprintf("%v", *c.PaddleOCRVLCloudUseChartRecognition)
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// Value implements the driver.Valuer interface for ParserEngineConfig
func (c *ParserEngineConfig) Value() (driver.Value, error) {
	if c == nil {
		return nil, nil
	}
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface for ParserEngineConfig
func (c *ParserEngineConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, c)
}

// StorageEngineConfig holds platform-wide storage engine parameters for Local,
// MinIO, COS, TOS, S3, OSS, KS3, and OBS.
// Knowledge bases select which provider to use; parameters are read from here.
type StorageEngineConfig struct {
	DefaultProvider string             `json:"default_provider"` // "local", "minio", "cos", "tos", "s3", "oss", "ks3", "obs"
	Local           *LocalEngineConfig `json:"local,omitempty"`
	MinIO           *MinIOEngineConfig `json:"minio,omitempty"`
	COS             *COSEngineConfig   `json:"cos,omitempty"`
	TOS             *TOSEngineConfig   `json:"tos,omitempty"`
	S3              *S3EngineConfig    `json:"s3,omitempty"`
	OSS             *OSSEngineConfig   `json:"oss,omitempty"`
	KS3             *KS3EngineConfig   `json:"ks3,omitempty"`
	OBS             *OBSEngineConfig   `json:"obs,omitempty"`
}

// LocalEngineConfig is for local file system storage (single-machine deployment only).
type LocalEngineConfig struct {
	PathPrefix string `json:"path_prefix"`
}

// MinIOEngineConfig is for MinIO/S3-compatible object storage.
// Mode "docker" uses env vars for endpoint/credentials; "remote" uses the fields below.
type MinIOEngineConfig struct {
	Mode            string `json:"mode"` // "docker" or "remote"
	Endpoint        string `json:"endpoint"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	BucketName      string `json:"bucket_name"`
	UseSSL          bool   `json:"use_ssl"`
	PathPrefix      string `json:"path_prefix"`
}

// COSEngineConfig is for Tencent Cloud COS.
type COSEngineConfig struct {
	SecretID   string `json:"secret_id"`
	SecretKey  string `json:"secret_key"`
	Region     string `json:"region"`
	BucketName string `json:"bucket_name"`
	AppID      string `json:"app_id"`
	PathPrefix string `json:"path_prefix"`
}

// TOSEngineConfig is for Volcengine TOS (火山引擎对象存储).
type TOSEngineConfig struct {
	Endpoint   string `json:"endpoint"`
	Region     string `json:"region"`
	AccessKey  string `json:"access_key"`
	SecretKey  string `json:"secret_key"`
	BucketName string `json:"bucket_name"`
	PathPrefix string `json:"path_prefix"`
}

// S3EngineConfig is for AWS S3 and S3-compatible object storage.
type S3EngineConfig struct {
	Endpoint       string `json:"endpoint"`
	Region         string `json:"region"`
	AccessKey      string `json:"access_key"`
	SecretKey      string `json:"secret_key"`
	BucketName     string `json:"bucket_name"`
	PathPrefix     string `json:"path_prefix"`
	UseSSL         bool   `json:"use_ssl"`
	ForcePathStyle bool   `json:"force_path_style"`
}

// OSSEngineConfig is for Alibaba Cloud OSS (对象存储服务).
type OSSEngineConfig struct {
	Endpoint       string `json:"endpoint"`
	Region         string `json:"region"`
	AccessKey      string `json:"access_key"`
	SecretKey      string `json:"secret_key"`
	BucketName     string `json:"bucket_name"`
	PathPrefix     string `json:"path_prefix"`
	UseTempBucket  bool   `json:"use_temp_bucket"`
	TempBucketName string `json:"temp_bucket_name"`
	TempRegion     string `json:"temp_region"`
}

// KS3EngineConfig is for Kingsoft Cloud KS3 object storage.
type KS3EngineConfig struct {
	Endpoint   string `json:"endpoint"`
	Region     string `json:"region"`
	AccessKey  string `json:"access_key"`
	SecretKey  string `json:"secret_key"`
	BucketName string `json:"bucket_name"`
	PathPrefix string `json:"path_prefix"`
}

// OBSEngineConfig is for Huawei Cloud OBS (对象存储服务).
type OBSEngineConfig struct {
	Endpoint   string `json:"endpoint"`
	Region     string `json:"region"`
	AccessKey  string `json:"access_key"`
	SecretKey  string `json:"secret_key"`
	BucketName string `json:"bucket_name"`
	PathPrefix string `json:"path_prefix"`
	UseSSL     bool   `json:"use_ssl"`
}

// Value implements the driver.Valuer interface for StorageEngineConfig
func (c *StorageEngineConfig) Value() (driver.Value, error) {
	if c == nil {
		return nil, nil
	}
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface for StorageEngineConfig
func (c *StorageEngineConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, c)
}

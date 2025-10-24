package entity

// S3BucketLifecycleStatus representa o status de lifecycle/versão/IT por bucket,
// além de sinalizações de criptografia e exposição pública.
type S3BucketLifecycleStatus struct {
	Bucket string `json:"bucket"`
	Region string `json:"region"`

	// Lifecycle
	HasLifecycle           bool `json:"has_lifecycle"`
	LifecycleRulesCount    int  `json:"lifecycle_rules_count"`
	HasNoncurrentLifecycle bool `json:"has_noncurrent_lifecycle"`

	// Versioning
	VersioningEnabled   bool `json:"versioning_enabled"`
	VersioningMFADelete bool `json:"versioning_mfa_delete,omitempty"`

	// Intelligent-Tiering
	HasIntelligentTieringCfg bool `json:"has_intelligent_tiering_cfg"`
	// Quando a configuração de IT vier via Lifecycle (transition para INTELLIGENT_TIERING)
	HasIntelligentTieringViaLifecycle bool `json:"has_intelligent_tiering_via_lifecycle"`

	// Criptografia padrão (default encryption)
	DefaultEncryptionEnabled bool   `json:"default_encryption_enabled"`
	DefaultEncryptionAlgo    string `json:"default_encryption_algo,omitempty"`
	DefaultEncryptionKMSKey  string `json:"default_encryption_kms_key,omitempty"`

	// Public Access Block (bucket-level)
	BlockPublicAcls       bool `json:"block_public_acls"`
	BlockPublicPolicy     bool `json:"block_public_policy"`
	IgnorePublicAcls      bool `json:"ignore_public_acls"`
	RestrictPublicBuckets bool `json:"restrict_public_buckets"`

	// Sinalização de possível exposição pública (heurística)
	IsPublic bool `json:"is_public"`
}

// S3LifecycleAudit agrega os achados para apresentação/exportação.
type S3LifecycleAudit struct {
	Profile   string `json:"profile"`
	AccountID string `json:"account_id"`

	TotalBuckets                        int `json:"total_buckets"`
	NoLifecycleCount                    int `json:"no_lifecycle_count"`
	VersionedWithoutNoncurrentLifecycle int `json:"versioned_without_noncurrent_lifecycle"`
	NoIntelligentTieringCount           int `json:"no_intelligent_tiering_count"`

	// Novas métricas
	NoDefaultEncryptionCount int `json:"no_default_encryption_count"`
	PublicRiskCount          int `json:"public_risk_count"`

	// Amostras para visual rápida (ordenadas alfabeticamente)
	SampleNoLifecycle                    []S3BucketLifecycleStatus `json:"sample_no_lifecycle"`
	SampleVersionedWithoutNoncurrentRule []S3BucketLifecycleStatus `json:"sample_versioned_without_noncurrent_rule"`
	SampleNoIntelligentTiering           []S3BucketLifecycleStatus `json:"sample_no_intelligent_tiering"`
	SampleNoDefaultEncryption            []S3BucketLifecycleStatus `json:"sample_no_default_encryption"`
	SamplePublicRisk                     []S3BucketLifecycleStatus `json:"sample_public_risk"`

	// Distribuição por região (buckets afetados por região)
	RegionsNoLifecycle map[string]int `json:"regions_no_lifecycle,omitempty"`

	RecommendedMessage string `json:"recommended_message,omitempty"`
}

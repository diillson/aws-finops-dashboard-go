package entity

// CloudWatchLogGroupInfo representa um log group e sua configuração de retenção e tamanho.
type CloudWatchLogGroupInfo struct {
	GroupName     string `json:"group_name"`
	Region        string `json:"region"`
	RetentionDays int    `json:"retention_days"` // 0 => Never expire
	StoredBytes   int64  `json:"stored_bytes"`
}

// CloudWatchLogsAudit agrega os achados por perfil/conta para export/visualização.
type CloudWatchLogsAudit struct {
	Profile            string                   `json:"profile"`
	AccountID          string                   `json:"account_id"`
	NoRetentionCount   int                      `json:"no_retention_count"`
	NoRetentionTopN    []CloudWatchLogGroupInfo `json:"no_retention_top_n"`
	TotalStoredGB      float64                  `json:"total_stored_gb"`
	RecommendedMessage string                   `json:"recommended_message,omitempty"`
}

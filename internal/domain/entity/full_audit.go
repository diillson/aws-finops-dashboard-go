package entity

// FullAuditReport agrega todos os relatórios de auditoria para um perfil/conta.
// Usamos ponteiros para que, se uma sub-auditoria falhar, ela possa ser nula sem quebrar o resto.
type FullAuditReport struct {
	Profile   string `json:"profile"`
	AccountID string `json:"account_id"`

	// Sub-relatórios
	MainAudit        *AuditData           `json:"main_audit,omitempty"`
	TransferAudit    *DataTransferReport  `json:"transfer_audit,omitempty"`
	LogsAudit        *CloudWatchLogsAudit `json:"logs_audit,omitempty"`
	S3Audit          *S3LifecycleAudit    `json:"s3_audit,omitempty"`
	CommitmentsAudit *CommitmentsReport   `json:"commitments_audit,omitempty"`
}

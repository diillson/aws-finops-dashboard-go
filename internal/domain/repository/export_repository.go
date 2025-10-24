package repository

import (
	"github.com/diillson/aws-finops-dashboard-go/internal/domain/entity"
)

type ExportRepository interface {
	ExportToCSV(data []entity.ProfileData, filename string, outputDir string, previousPeriodDates, currentPeriodDates string) (string, error)
	ExportToJSON(data []entity.ProfileData, filename string, outputDir string) (string, error)
	ExportToPDF(data []entity.ProfileData, filename string, outputDir string, previousPeriodDates, currentPeriodDates string) (string, error)

	ExportAuditReportToPDF(auditData []entity.AuditData, filename string, outputDir string) (string, error)
	ExportAuditReportToCSV(auditData []entity.AuditData, filename string, outputDir string) (string, error)
	ExportAuditReportToJSON(auditData []entity.AuditData, filename string, outputDir string) (string, error)

	// Data Transfer
	ExportTransferReportToCSV(reports []entity.DataTransferReport, filename, outputDir string) (string, error)
	ExportTransferReportToJSON(reports []entity.DataTransferReport, filename, outputDir string) (string, error)
	ExportTransferReportToPDF(reports []entity.DataTransferReport, filename, outputDir string) (string, error)

	// Logs Audit
	ExportLogsAuditToCSV(audits []entity.CloudWatchLogsAudit, filename, outputDir string) (string, error)
	ExportLogsAuditToJSON(audits []entity.CloudWatchLogsAudit, filename, outputDir string) (string, error)
	ExportLogsAuditToPDF(audits []entity.CloudWatchLogsAudit, filename, outputDir string) (string, error)

	// S3 Lifecycle Audit
	ExportS3LifecycleAuditToCSV(audits []entity.S3LifecycleAudit, filename, outputDir string) (string, error)
	ExportS3LifecycleAuditToJSON(audits []entity.S3LifecycleAudit, filename, outputDir string) (string, error)
	ExportS3LifecycleAuditToPDF(audits []entity.S3LifecycleAudit, filename, outputDir string) (string, error)
}

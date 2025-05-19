package repository

import (
	"github.com/diillson/aws-finops-dashboard-go/internal/domain/entity"
)

// ExportRepository defines the interface for exporting data to different formats.
type ExportRepository interface {
	ExportToCSV(data []entity.ProfileData, filename string, outputDir string, previousPeriodDates, currentPeriodDates string) (string, error)
	ExportToJSON(data []entity.ProfileData, filename string, outputDir string) (string, error)
	ExportToPDF(data []entity.ProfileData, filename string, outputDir string, previousPeriodDates, currentPeriodDates string) (string, error)

	ExportAuditReportToPDF(auditData []entity.AuditData, filename string, outputDir string) (string, error)
	ExportAuditReportToCSV(auditData []entity.AuditData, filename string, outputDir string) (string, error)
	ExportAuditReportToJSON(auditData []entity.AuditData, filename string, outputDir string) (string, error)
}

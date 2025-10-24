package export

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/diillson/aws-finops-dashboard-go/internal/domain/entity"
	"github.com/diillson/aws-finops-dashboard-go/internal/domain/repository"
	"github.com/jung-kurt/gofpdf"
)

// ExportRepositoryImpl implementa o ExportRepository.
type ExportRepositoryImpl struct{}

// NewExportRepository cria uma nova implementação do ExportRepository.
func NewExportRepository() repository.ExportRepository {
	return &ExportRepositoryImpl{}
}

// --- Funções de Exportação do Dashboard de Custos ---

func (r *ExportRepositoryImpl) ExportToCSV(data []entity.ProfileData, filename, outputDir, previousPeriodDates, currentPeriodDates string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "csv")
	if err != nil {
		return "", err
	}

	file, err := os.Create(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error creating CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	headers := []string{
		"CLI Profile", "AWS Account ID",
		fmt.Sprintf("Cost for period (%s)", previousPeriodDates),
		fmt.Sprintf("Cost for period (%s)", currentPeriodDates),
		"Cost By Service", "Budget Status", "EC2 Instances",
	}
	writer.Write(headers)

	for _, row := range data {
		servicesData := ""
		for _, sc := range row.ServiceCosts {
			servicesData += fmt.Sprintf("%s: $%.2f\n", sc.ServiceName, sc.Cost)
			for _, sub := range sc.SubCosts {
				servicesData += fmt.Sprintf("  - %s: $%.2f\n", sub.ServiceName, sub.Cost)
			}
		}

		record := []string{
			row.Profile,
			row.AccountID,
			fmt.Sprintf("$%.2f", row.LastMonth),
			fmt.Sprintf("$%.2f", row.CurrentMonth),
			strings.TrimSpace(servicesData),
			strings.Join(row.BudgetInfo, "\n"),
			// Remove quaisquer códigos ANSI que tenham “sobrado” em strings (por segurança)
			cleanRichTags(strings.Join(row.EC2SummaryFormatted, "\n")),
		}
		writer.Write(record)
	}

	return filepath.Abs(outputFilename)
}

func (r *ExportRepositoryImpl) ExportToJSON(data []entity.ProfileData, filename, outputDir string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "json")
	if err != nil {
		return "", err
	}

	file, err := os.Create(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error creating JSON file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return "", fmt.Errorf("error encoding JSON data: %w", err)
	}

	return filepath.Abs(outputFilename)
}

func (r *ExportRepositoryImpl) ExportToPDF(data []entity.ProfileData, filename, outputDir, previousPeriodDates, currentPeriodDates string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "pdf")
	if err != nil {
		return "", err
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	headerColor := [3]int{40, 40, 40}
	headerTextColor := [3]int{255, 255, 255}
	sectionTitleColor := [3]int{0, 0, 0}
	bodyTextColor := [3]int{50, 50, 50}
	lineColor := [3]int{200, 200, 200}

	drawSection := func(title string, content string) {
		if content == "" {
			return
		}
		pdf.SetFont("Arial", "B", 12)
		pdf.SetTextColor(sectionTitleColor[0], sectionTitleColor[1], sectionTitleColor[2])
		pdf.Cell(0, 8, title)
		pdf.Ln(7)

		pdf.SetDrawColor(lineColor[0], lineColor[1], lineColor[2])
		pdf.Line(pdf.GetX(), pdf.GetY(), pdf.GetX()+190, pdf.GetY())
		pdf.Ln(4)

		pdf.SetFont("Arial", "", 10)
		pdf.SetTextColor(bodyTextColor[0], bodyTextColor[1], bodyTextColor[2])
		pdf.MultiCell(190, 5, tr(content), "", "L", false)
		pdf.Ln(8)
	}

	for i, rowData := range data {
		pdf.AddPage()

		pdf.SetFillColor(headerColor[0], headerColor[1], headerColor[2])
		pdf.SetTextColor(headerTextColor[0], headerTextColor[1], headerTextColor[2])
		pdf.SetFont("Arial", "B", 14)
		profileName := rowData.Profile
		if len(profileName) > 80 {
			profileName = profileName[:77] + "..."
		}
		pdf.CellFormat(0, 12, tr(fmt.Sprintf("  %s", profileName)), "", 1, "L", true, 0, "")

		pdf.SetFont("Arial", "", 10)
		pdf.SetFillColor(240, 240, 240)
		pdf.SetTextColor(bodyTextColor[0], bodyTextColor[1], bodyTextColor[2])
		pdf.CellFormat(0, 8, tr(fmt.Sprintf("  Account ID: %s", rowData.AccountID)), "", 1, "L", true, 0, "")
		pdf.Ln(10)

		pdf.SetFont("Arial", "B", 12)
		pdf.SetTextColor(sectionTitleColor[0], sectionTitleColor[1], sectionTitleColor[2])
		pdf.Cell(0, 8, "Cost Summary")
		pdf.Ln(7)
		pdf.SetDrawColor(lineColor[0], lineColor[1], lineColor[2])
		pdf.Line(pdf.GetX(), pdf.GetY(), pdf.GetX()+190, pdf.GetY())
		pdf.Ln(4)

		costTableWidth := 95.0
		pdf.SetFont("Arial", "B", 10)
		pdf.SetTextColor(bodyTextColor[0], bodyTextColor[1], bodyTextColor[2])
		pdf.CellFormat(costTableWidth, 7, tr(rowData.PreviousPeriodName), "B", 0, "L", false, 0, "")
		pdf.CellFormat(costTableWidth, 7, tr(rowData.CurrentPeriodName), "B", 1, "L", false, 0, "")

		pdf.SetFont("Arial", "", 8)
		pdf.SetTextColor(100, 100, 100)
		pdf.CellFormat(costTableWidth, 5, tr(previousPeriodDates), "", 0, "L", false, 0, "")
		pdf.CellFormat(costTableWidth, 5, tr(currentPeriodDates), "", 1, "L", false, 0, "")
		pdf.SetTextColor(bodyTextColor[0], bodyTextColor[1], bodyTextColor[2])

		pdf.SetFont("Arial", "B", 16)
		pdf.CellFormat(costTableWidth, 12, tr(fmt.Sprintf("$%.2f", rowData.LastMonth)), "", 0, "L", false, 0, "")

		changeText := ""
		originalTextColorR, originalTextColorG, originalTextColorB := pdf.GetTextColor()
		if rowData.PercentChangeInCost != nil {
			val := *rowData.PercentChangeInCost
			if val > 0.01 {
				pdf.SetTextColor(192, 0, 0)
				changeText = fmt.Sprintf("  (▲ +%.2f%%)", val)
			} else if val < -0.01 {
				pdf.SetTextColor(0, 128, 0)
				changeText = fmt.Sprintf("  (▼ %.2f%%)", val)
			} else {
				changeText = "  (0.00%)"
			}
		}

		pdf.SetFont("Arial", "B", 16)
		valueStr := fmt.Sprintf("$%.2f", rowData.CurrentMonth)
		pdf.Cell(pdf.GetStringWidth(valueStr), 12, tr(valueStr))

		pdf.SetFont("Arial", "", 10)
		pdf.CellFormat(costTableWidth-pdf.GetStringWidth(valueStr), 12, tr(changeText), "", 1, "L", false, 0, "")

		pdf.SetTextColor(originalTextColorR, originalTextColorG, originalTextColorB)
		pdf.Ln(10)

		serviceCostsStr := ""
		for _, sc := range rowData.ServiceCosts {
			serviceCostsStr += fmt.Sprintf("%s: $%.2f\n", sc.ServiceName, sc.Cost)
			for _, sub := range sc.SubCosts {
				serviceCostsStr += fmt.Sprintf("  └─ %s: $%.2f\n", sub.ServiceName, sub.Cost)
			}
		}
		drawSection("Cost By Service", strings.TrimSpace(serviceCostsStr))
		drawSection("Budget Status", strings.Join(rowData.BudgetInfo, "\n\n"))
		drawSection("EC2 Instances", cleanRichTags(strings.Join(rowData.EC2SummaryFormatted, "\n")))

		pdf.SetY(-15)
		pdf.SetFont("Arial", "I", 8)
		pdf.SetTextColor(128, 128, 128)
		footerText := fmt.Sprintf("Generated by AWS FinOps Dashboard (Go) | %s", time.Now().Format("2006-01-02"))
		pdf.CellFormat(0, 10, tr(footerText), "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 10, tr(fmt.Sprintf("Page %d", i+1)), "", 0, "R", false, 0, "")
	}

	if err := pdf.OutputFileAndClose(outputFilename); err != nil {
		return "", fmt.Errorf("error writing PDF file: %w", err)
	}

	return filepath.Abs(outputFilename)
}

// --- Funções de Exportação do Relatório de Auditoria ---

func (r *ExportRepositoryImpl) ExportAuditReportToCSV(auditData []entity.AuditData, filename, outputDir string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "csv")
	if err != nil {
		return "", err
	}

	file, err := os.Create(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error creating audit CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	headers := []string{
		"Profile",
		"Account ID",
		"Budget Alerts",
		"High-Cost NAT Gateways",
		"Unused VPC Endpoints",
		"Idle Load Balancers",
		"Stopped EC2 Instances",
		"Unused EBS Volumes",
		"Unused Elastic IPs",
		"Untagged Resources",
	}
	if err := writer.Write(headers); err != nil {
		return "", fmt.Errorf("error writing CSV header: %w", err)
	}

	for _, row := range auditData {
		record := []string{
			row.Profile,
			row.AccountID,
			cleanRichTags(row.BudgetAlerts),
			cleanRichTags(row.NatGatewayCosts),
			cleanRichTags(row.UnusedVpcEndpoints),
			cleanRichTags(row.IdleLoadBalancers),
			cleanRichTags(row.StoppedInstances),
			cleanRichTags(row.UnusedVolumes),
			cleanRichTags(row.UnusedEIPs),
			cleanRichTags(row.UntaggedResources),
		}
		if err := writer.Write(record); err != nil {
			return "", fmt.Errorf("error writing CSV record: %w", err)
		}
	}

	return filepath.Abs(outputFilename)
}

func (r *ExportRepositoryImpl) ExportAuditReportToJSON(auditData []entity.AuditData, filename, outputDir string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "json")
	if err != nil {
		return "", err
	}

	cleanData := make([]entity.AuditData, len(auditData))
	for i, row := range auditData {
		cleanData[i] = entity.AuditData{
			Profile:            row.Profile,
			AccountID:          row.AccountID,
			BudgetAlerts:       cleanRichTags(row.BudgetAlerts),
			NatGatewayCosts:    cleanRichTags(row.NatGatewayCosts),
			IdleLoadBalancers:  cleanRichTags(row.IdleLoadBalancers),
			StoppedInstances:   cleanRichTags(row.StoppedInstances),
			UnusedVolumes:      cleanRichTags(row.UnusedVolumes),
			UnusedEIPs:         cleanRichTags(row.UnusedEIPs),
			UntaggedResources:  cleanRichTags(row.UntaggedResources),
			UnusedVpcEndpoints: cleanRichTags(row.UnusedVpcEndpoints),
		}
	}

	file, err := os.Create(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error creating audit JSON file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cleanData); err != nil {
		return "", fmt.Errorf("error encoding audit JSON data: %w", err)
	}

	return filepath.Abs(outputFilename)
}

func (r *ExportRepositoryImpl) ExportAuditReportToPDF(auditData []entity.AuditData, filename, outputDir string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "pdf")
	if err != nil {
		return "", err
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	for i, row := range auditData {
		pdf.AddPage()
		headerColor := [3]int{192, 0, 0}
		headerTextColor := [3]int{255, 255, 255}
		sectionTitleColor := [3]int{0, 0, 0}
		bodyTextColor := [3]int{50, 50, 50}
		lineColor := [3]int{200, 200, 200}

		drawSection := func(title string, content string) {
			content = cleanRichTags(content)
			if content == "" || content == "None" {
				return
			}
			pdf.SetFont("Arial", "B", 12)
			pdf.SetTextColor(sectionTitleColor[0], sectionTitleColor[1], sectionTitleColor[2])
			pdf.Cell(0, 8, tr(title))
			pdf.Ln(7)

			pdf.SetDrawColor(lineColor[0], lineColor[1], lineColor[2])
			pdf.Line(pdf.GetX(), pdf.GetY(), pdf.GetX()+190, pdf.GetY())
			pdf.Ln(4)

			pdf.SetFont("Arial", "", 10)
			pdf.SetTextColor(bodyTextColor[0], bodyTextColor[1], bodyTextColor[2])
			pdf.MultiCell(190, 5, tr(content), "", "L", false)
			pdf.Ln(8)
		}

		// Cabeçalho
		pdf.SetFillColor(headerColor[0], headerColor[1], headerColor[2])
		pdf.SetTextColor(headerTextColor[0], headerTextColor[1], headerTextColor[2])
		pdf.SetFont("Arial", "B", 14)
		pdf.CellFormat(0, 12, tr(fmt.Sprintf("  Audit Report: %s", row.Profile)), "", 1, "L", true, 0, "")
		pdf.SetFont("Arial", "", 10)
		pdf.SetFillColor(240, 240, 240)
		pdf.SetTextColor(bodyTextColor[0], bodyTextColor[1], bodyTextColor[2])
		pdf.CellFormat(0, 8, tr(fmt.Sprintf("  Account ID: %s", row.AccountID)), "", 1, "L", true, 0, "")
		pdf.Ln(10)

		// Seções da Auditoria — ordem consistente com o terminal
		drawSection("Budget Alerts", row.BudgetAlerts)
		drawSection("High-Cost NAT Gateways", row.NatGatewayCosts)
		drawSection("Unused VPC Endpoints", row.UnusedVpcEndpoints)
		drawSection("Idle Load Balancers", row.IdleLoadBalancers)
		drawSection("Stopped EC2 Instances", row.StoppedInstances)
		drawSection("Unused EBS Volumes", row.UnusedVolumes)
		drawSection("Unused Elastic IPs", row.UnusedEIPs)
		drawSection("Untagged Resources", row.UntaggedResources)

		// Rodapé
		pdf.SetY(-15)
		pdf.SetFont("Arial", "I", 8)
		pdf.SetTextColor(128, 128, 128)
		footerText := fmt.Sprintf("Audit Report | %s", time.Now().Format("2006-01-02"))
		pdf.CellFormat(0, 10, tr(footerText), "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 10, tr(fmt.Sprintf("Page %d", i+1)), "", 0, "R", false, 0, "")
	}

	if err := pdf.OutputFileAndClose(outputFilename); err != nil {
		return "", fmt.Errorf("error writing audit PDF file: %w", err)
	}

	return filepath.Abs(outputFilename)
}

// --- Funções Auxiliares ---

// generateFilename cria um nome de arquivo único com timestamp e garante que o diretório exista.
func generateFilename(base, dir, ext string) (string, error) {
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("could not get current working directory: %w", err)
		}
		dir = cwd
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("error creating output directory '%s': %w", dir, err)
	}
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.%s", base, timestamp, ext)
	return filepath.Join(dir, filename), nil
}

// Regex para limpar formatação pterm (rich tags) e sequências ANSI de cor/estilo.
var richTagRegex = regexp.MustCompile(`\[/?([a-zA-Z]+|#[0-9a-fA-F]{6})\]`)
var ansiRegex = regexp.MustCompile(`\x1B\[[0-9;]*[A-Za-z]`)

// cleanRichTags remove tags de formatação do pterm e sequências ANSI.
func cleanRichTags(text string) string {
	text = richTagRegex.ReplaceAllString(text, "")
	text = ansiRegex.ReplaceAllString(text, "")
	return text
}

func (r *ExportRepositoryImpl) ExportTransferReportToCSV(reports []entity.DataTransferReport, filename, outputDir string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "csv")
	if err != nil {
		return "", err
	}

	file, err := os.Create(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error creating transfer CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	headers := []string{
		"Account ID", "Period", "Total",
		"Internet", "Inter-Region", "Cross-AZ/Regional", "NAT Gateway", "Other",
		"Top Lines", // formatado como várias linhas em uma célula
	}
	if err := writer.Write(headers); err != nil {
		return "", fmt.Errorf("error writing CSV header: %w", err)
	}

	for _, rep := range reports {
		period := fmt.Sprintf("%s to %s", rep.PeriodStart.Format("2006-01-02"), rep.PeriodEnd.Format("2006-01-02"))

		getCat := func(name string) float64 {
			for _, c := range rep.Categories {
				if c.Category == name {
					return c.Cost
				}
			}
			return 0
		}

		var topLines []string
		for _, l := range rep.TopLines {
			topLines = append(topLines, fmt.Sprintf("%s | %s: $%.2f", l.Service, l.UsageType, l.Cost))
		}

		record := []string{
			rep.AccountID,
			period,
			fmt.Sprintf("$%.2f", rep.Total),
			fmt.Sprintf("$%.2f", getCat("Internet")),
			fmt.Sprintf("$%.2f", getCat("Inter-Region")),
			fmt.Sprintf("$%.2f", getCat("Cross-AZ/Regional")),
			fmt.Sprintf("$%.2f", getCat("NAT Gateway")),
			fmt.Sprintf("$%.2f", getCat("Other")),
			cleanRichTags(strings.Join(topLines, "\n")),
		}

		if err := writer.Write(record); err != nil {
			return "", fmt.Errorf("error writing CSV record: %w", err)
		}
	}

	return filepath.Abs(outputFilename)
}

func (r *ExportRepositoryImpl) ExportTransferReportToJSON(reports []entity.DataTransferReport, filename, outputDir string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "json")
	if err != nil {
		return "", err
	}

	file, err := os.Create(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error creating transfer JSON file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(reports); err != nil {
		return "", fmt.Errorf("error encoding transfer JSON data: %w", err)
	}

	return filepath.Abs(outputFilename)
}

func (r *ExportRepositoryImpl) ExportTransferReportToPDF(reports []entity.DataTransferReport, filename, outputDir string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "pdf")
	if err != nil {
		return "", err
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	for i, rep := range reports {
		pdf.AddPage()
		headerColor := [3]int{0, 102, 204}
		headerTextColor := [3]int{255, 255, 255}
		sectionTitleColor := [3]int{0, 0, 0}
		bodyTextColor := [3]int{50, 50, 50}
		lineColor := [3]int{200, 200, 200}

		drawSection := func(title string, content string) {
			content = cleanRichTags(content)
			if strings.TrimSpace(content) == "" {
				return
			}
			pdf.SetFont("Arial", "B", 12)
			pdf.SetTextColor(sectionTitleColor[0], sectionTitleColor[1], sectionTitleColor[2])
			pdf.Cell(0, 8, tr(title))
			pdf.Ln(7)

			pdf.SetDrawColor(lineColor[0], lineColor[1], lineColor[2])
			pdf.Line(pdf.GetX(), pdf.GetY(), pdf.GetX()+190, pdf.GetY())
			pdf.Ln(4)

			pdf.SetFont("Arial", "", 10)
			pdf.SetTextColor(bodyTextColor[0], bodyTextColor[1], bodyTextColor[2])
			pdf.MultiCell(190, 5, tr(content), "", "L", false)
			pdf.Ln(8)
		}

		// Cabeçalho
		pdf.SetFillColor(headerColor[0], headerColor[1], headerColor[2])
		pdf.SetTextColor(headerTextColor[0], headerTextColor[1], headerTextColor[2])
		pdf.SetFont("Arial", "B", 14)
		pdf.CellFormat(0, 12, tr("  Data Transfer Deep Dive"), "", 1, "L", true, 0, "")
		pdf.SetFont("Arial", "", 10)
		pdf.SetFillColor(240, 240, 240)
		pdf.SetTextColor(bodyTextColor[0], bodyTextColor[1], bodyTextColor[2])
		pdf.CellFormat(0, 8, tr(fmt.Sprintf("  Account ID: %s", rep.AccountID)), "", 1, "L", true, 0, "")
		pdf.CellFormat(0, 8, tr(fmt.Sprintf("  Period: %s to %s", rep.PeriodStart.Format("2006-01-02"), rep.PeriodEnd.Format("2006-01-02"))), "", 1, "L", true, 0, "")
		pdf.Ln(8)

		// Resumo por categoria
		var b strings.Builder
		b.WriteString(fmt.Sprintf("Total: $%.2f\n\n", rep.Total))
		// Ordena categorias por custo desc já vem ordenado do repo, mas garantimos:
		cats := make([]entity.DataTransferCategoryCost, len(rep.Categories))
		copy(cats, rep.Categories)
		sort.Slice(cats, func(i, j int) bool { return cats[i].Cost > cats[j].Cost })
		for _, c := range cats {
			b.WriteString(fmt.Sprintf("%s: $%.2f\n", c.Category, c.Cost))
		}
		drawSection("Category Summary", b.String())

		// Top Lines
		if len(rep.TopLines) > 0 {
			var tl strings.Builder
			limit := len(rep.TopLines)
			if limit > 15 {
				limit = 15
			}
			for i := 0; i < limit; i++ {
				l := rep.TopLines[i]
				tl.WriteString(fmt.Sprintf("%s | %s: $%.2f\n", l.Service, l.UsageType, l.Cost))
			}
			if len(rep.TopLines) > limit {
				tl.WriteString(fmt.Sprintf("... (+%d more)\n", len(rep.TopLines)-limit))
			}
			drawSection("Top Lines", tl.String())
		}

		// Rodapé
		pdf.SetY(-15)
		pdf.SetFont("Arial", "I", 8)
		pdf.SetTextColor(128, 128, 128)
		footerText := fmt.Sprintf("Data Transfer | %s", time.Now().Format("2006-01-02"))
		pdf.CellFormat(0, 10, tr(footerText), "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 10, tr(fmt.Sprintf("Page %d", i+1)), "", 0, "R", false, 0, "")
	}

	if err := pdf.OutputFileAndClose(outputFilename); err != nil {
		return "", fmt.Errorf("error writing transfer PDF file: %w", err)
	}
	return filepath.Abs(outputFilename)
}

func (r *ExportRepositoryImpl) ExportLogsAuditToCSV(audits []entity.CloudWatchLogsAudit, filename, outputDir string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "csv")
	if err != nil {
		return "", err
	}

	f, err := os.Create(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error creating logs audit CSV file: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	headers := []string{
		"Profile", "Account ID", "No Retention (count)", "Total Stored (GB)", "Top No-Retention Groups",
	}
	if err := w.Write(headers); err != nil {
		return "", fmt.Errorf("error writing CSV header: %w", err)
	}

	for _, a := range audits {
		var top []string
		for _, lg := range a.NoRetentionTopN {
			gb := float64(lg.StoredBytes) / (1024.0 * 1024.0 * 1024.0)
			top = append(top, fmt.Sprintf("%s | %s | %.2f GB", lg.Region, lg.GroupName, gb))
		}
		record := []string{
			a.Profile,
			a.AccountID,
			fmt.Sprintf("%d", a.NoRetentionCount),
			fmt.Sprintf("%.2f", a.TotalStoredGB),
			cleanRichTags(strings.Join(top, "\n")),
		}
		if err := w.Write(record); err != nil {
			return "", fmt.Errorf("error writing CSV record: %w", err)
		}
	}
	return filepath.Abs(outputFilename)
}

func (r *ExportRepositoryImpl) ExportLogsAuditToJSON(audits []entity.CloudWatchLogsAudit, filename, outputDir string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "json")
	if err != nil {
		return "", err
	}

	f, err := os.Create(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error creating logs audit JSON file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(audits); err != nil {
		return "", fmt.Errorf("error encoding logs audit JSON: %w", err)
	}
	return filepath.Abs(outputFilename)
}

func (r *ExportRepositoryImpl) ExportLogsAuditToPDF(audits []entity.CloudWatchLogsAudit, filename, outputDir string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "pdf")
	if err != nil {
		return "", err
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	for i, a := range audits {
		pdf.AddPage()
		headerColor := [3]int{51, 51, 51}
		headerTextColor := [3]int{255, 255, 255}
		sectionTitleColor := [3]int{0, 0, 0}
		bodyTextColor := [3]int{50, 50, 50}
		lineColor := [3]int{200, 200, 200}

		drawSection := func(title string, content string) {
			content = cleanRichTags(content)
			if strings.TrimSpace(content) == "" {
				return
			}
			pdf.SetFont("Arial", "B", 12)
			pdf.SetTextColor(sectionTitleColor[0], sectionTitleColor[1], sectionTitleColor[2])
			pdf.Cell(0, 8, tr(title))
			pdf.Ln(7)
			pdf.SetDrawColor(lineColor[0], lineColor[1], lineColor[2])
			pdf.Line(pdf.GetX(), pdf.GetY(), pdf.GetX()+190, pdf.GetY())
			pdf.Ln(4)
			pdf.SetFont("Arial", "", 10)
			pdf.SetTextColor(bodyTextColor[0], bodyTextColor[1], bodyTextColor[2])
			pdf.MultiCell(190, 5, tr(content), "", "L", false)
			pdf.Ln(8)
		}

		// Header
		pdf.SetFillColor(headerColor[0], headerColor[1], headerColor[2])
		pdf.SetTextColor(headerTextColor[0], headerTextColor[1], headerTextColor[2])
		pdf.SetFont("Arial", "B", 14)
		pdf.CellFormat(0, 12, tr("  CloudWatch Logs Retention Audit"), "", 1, "L", true, 0, "")
		pdf.SetFont("Arial", "", 10)
		pdf.SetFillColor(240, 240, 240)
		pdf.SetTextColor(bodyTextColor[0], bodyTextColor[1], bodyTextColor[2])
		pdf.CellFormat(0, 8, tr(fmt.Sprintf("  Profile: %s", a.Profile)), "", 1, "L", true, 0, "")
		pdf.CellFormat(0, 8, tr(fmt.Sprintf("  Account ID: %s", a.AccountID)), "", 1, "L", true, 0, "")
		pdf.Ln(6)

		// Summary
		summary := fmt.Sprintf("No Retention (count): %d\nTotal Stored (GB): %.2f\n\nRecommendation: %s", a.NoRetentionCount, a.TotalStoredGB, a.RecommendedMessage)
		drawSection("Summary", summary)

		// Top Groups
		if len(a.NoRetentionTopN) > 0 {
			var b strings.Builder
			limit := len(a.NoRetentionTopN)
			if limit > 20 {
				limit = 20
			}
			for j := 0; j < limit; j++ {
				lg := a.NoRetentionTopN[j]
				gb := float64(lg.StoredBytes) / (1024.0 * 1024.0 * 1024.0)
				b.WriteString(fmt.Sprintf("%s | %s | %.2f GB\n", lg.Region, lg.GroupName, gb))
			}
			if len(a.NoRetentionTopN) > limit {
				b.WriteString(fmt.Sprintf("... (+%d more)\n", len(a.NoRetentionTopN)-limit))
			}
			drawSection("Top No-Retention Log Groups", b.String())
		}

		// Footer
		pdf.SetY(-15)
		pdf.SetFont("Arial", "I", 8)
		pdf.SetTextColor(128, 128, 128)
		pdf.CellFormat(0, 10, tr(fmt.Sprintf("CloudWatch Logs Audit | %s", time.Now().Format("2006-01-02"))), "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 10, tr(fmt.Sprintf("Page %d", i+1)), "", 0, "R", false, 0, "")
	}

	if err := pdf.OutputFileAndClose(outputFilename); err != nil {
		return "", fmt.Errorf("error writing logs audit PDF file: %w", err)
	}
	return filepath.Abs(outputFilename)
}

func (r *ExportRepositoryImpl) ExportS3LifecycleAuditToCSV(audits []entity.S3LifecycleAudit, filename, outputDir string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "csv")
	if err != nil {
		return "", err
	}

	f, err := os.Create(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error creating S3 lifecycle audit CSV file: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	headers := []string{
		"Profile", "Account ID",
		"Total Buckets", "No Lifecycle", "Versioned w/o Noncurrent Rule",
		"No Intelligent-Tiering", "No Default Encryption", "Public Risk",
		"Samples",
	}
	if err := w.Write(headers); err != nil {
		return "", fmt.Errorf("error writing CSV header: %w", err)
	}

	for _, a := range audits {
		var samples []string
		limit := func(list []entity.S3BucketLifecycleStatus, n int, label string) {
			m := len(list)
			if m > n {
				m = n
			}
			for i := 0; i < m; i++ {
				s := list[i]
				samples = append(samples, fmt.Sprintf("[%s] %s (%s)", label, s.Bucket, s.Region))
			}
			if len(list) > m {
				samples = append(samples, fmt.Sprintf("... (+%d more)", len(list)-m))
			}
		}
		limit(a.SampleNoLifecycle, 5, "NoLifecycle")
		limit(a.SampleVersionedWithoutNoncurrentRule, 5, "Versioned-NoNoncurrent")
		limit(a.SampleNoIntelligentTiering, 5, "No-IT")
		limit(a.SampleNoDefaultEncryption, 5, "No-Enc")
		limit(a.SamplePublicRisk, 5, "Public")

		record := []string{
			a.Profile,
			a.AccountID,
			fmt.Sprintf("%d", a.TotalBuckets),
			fmt.Sprintf("%d", a.NoLifecycleCount),
			fmt.Sprintf("%d", a.VersionedWithoutNoncurrentLifecycle),
			fmt.Sprintf("%d", a.NoIntelligentTieringCount),
			fmt.Sprintf("%d", a.NoDefaultEncryptionCount),
			fmt.Sprintf("%d", a.PublicRiskCount),
			cleanRichTags(strings.Join(samples, "\n")),
		}
		if err := w.Write(record); err != nil {
			return "", fmt.Errorf("error writing CSV record: %w", err)
		}
	}
	return filepath.Abs(outputFilename)
}

func (r *ExportRepositoryImpl) ExportS3LifecycleAuditToJSON(audits []entity.S3LifecycleAudit, filename, outputDir string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "json")
	if err != nil {
		return "", err
	}

	f, err := os.Create(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error creating S3 lifecycle audit JSON file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(audits); err != nil {
		return "", fmt.Errorf("error encoding S3 lifecycle audit JSON: %w", err)
	}
	return filepath.Abs(outputFilename)
}

func (r *ExportRepositoryImpl) ExportS3LifecycleAuditToPDF(audits []entity.S3LifecycleAudit, filename, outputDir string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "pdf")
	if err != nil {
		return "", err
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	for i, a := range audits {
		pdf.AddPage()
		headerColor := [3]int{0, 128, 128}
		headerTextColor := [3]int{255, 255, 255}
		sectionTitleColor := [3]int{0, 0, 0}
		bodyTextColor := [3]int{50, 50, 50}
		lineColor := [3]int{200, 200, 200}

		drawSection := func(title string, content string) {
			content = cleanRichTags(content)
			if strings.TrimSpace(content) == "" {
				return
			}
			pdf.SetFont("Arial", "B", 12)
			pdf.SetTextColor(sectionTitleColor[0], sectionTitleColor[1], sectionTitleColor[2])
			pdf.Cell(0, 8, tr(title))
			pdf.Ln(7)
			pdf.SetDrawColor(lineColor[0], lineColor[1], lineColor[2])
			pdf.Line(pdf.GetX(), pdf.GetY(), pdf.GetX()+190, pdf.GetY())
			pdf.Ln(4)
			pdf.SetFont("Arial", "", 10)
			pdf.SetTextColor(bodyTextColor[0], bodyTextColor[1], bodyTextColor[2])
			pdf.MultiCell(190, 5, tr(content), "", "L", false)
			pdf.Ln(8)
		}

		// Header
		pdf.SetFillColor(headerColor[0], headerColor[1], headerColor[2])
		pdf.SetTextColor(headerTextColor[0], headerTextColor[1], headerTextColor[2])
		pdf.SetFont("Arial", "B", 14)
		pdf.CellFormat(0, 12, tr("  S3 Lifecycle Audit"), "", 1, "L", true, 0, "")
		pdf.SetFont("Arial", "", 10)
		pdf.SetFillColor(240, 240, 240)
		pdf.SetTextColor(bodyTextColor[0], bodyTextColor[1], bodyTextColor[2])
		pdf.CellFormat(0, 8, tr(fmt.Sprintf("  Profile: %s", a.Profile)), "", 1, "L", true, 0, "")
		pdf.CellFormat(0, 8, tr(fmt.Sprintf("  Account ID: %s", a.AccountID)), "", 1, "L", true, 0, "")
		pdf.Ln(6)

		// Summary
		summary := fmt.Sprintf(
			"Total Buckets: %d\nNo Lifecycle: %d\nVersioned w/o Noncurrent Rule: %d\nNo Intelligent-Tiering: %d\nNo Default Encryption: %d\nPublic Risk: %d\n\nRecommendation: %s",
			a.TotalBuckets, a.NoLifecycleCount, a.VersionedWithoutNoncurrentLifecycle, a.NoIntelligentTieringCount, a.NoDefaultEncryptionCount, a.PublicRiskCount, a.RecommendedMessage,
		)
		drawSection("Summary", summary)

		// Regions breakdown (No Lifecycle)
		if len(a.RegionsNoLifecycle) > 0 {
			var b strings.Builder
			// ordenar regiões
			type kv struct {
				k string
				v int
			}
			var pairs []kv
			for k, v := range a.RegionsNoLifecycle {
				pairs = append(pairs, kv{k, v})
			}
			sort.Slice(pairs, func(i, j int) bool { return pairs[i].k < pairs[j].k })
			for _, p := range pairs {
				b.WriteString(fmt.Sprintf("%s: %d\n", p.k, p.v))
			}
			drawSection("No Lifecycle by Region", b.String())
		}

		// Samples
		if len(a.SampleNoLifecycle)+len(a.SampleVersionedWithoutNoncurrentRule)+len(a.SampleNoIntelligentTiering)+len(a.SampleNoDefaultEncryption)+len(a.SamplePublicRisk) > 0 {
			var s strings.Builder
			writeList := func(title string, list []entity.S3BucketLifecycleStatus, max int) {
				if len(list) == 0 {
					return
				}
				s.WriteString(title + ":\n")
				limit := len(list)
				if limit > max {
					limit = max
				}
				for i := 0; i < limit; i++ {
					b := list[i]
					s.WriteString(fmt.Sprintf("  - %s (%s)\n", b.Bucket, b.Region))
				}
				if len(list) > limit {
					s.WriteString(fmt.Sprintf("  ... (+%d more)\n", len(list)-limit))
				}
				s.WriteString("\n")
			}
			writeList("No Lifecycle", a.SampleNoLifecycle, 15)
			writeList("Versioned without Noncurrent Rule", a.SampleVersionedWithoutNoncurrentRule, 15)
			writeList("No Intelligent-Tiering", a.SampleNoIntelligentTiering, 15)
			writeList("No Default Encryption", a.SampleNoDefaultEncryption, 15)
			writeList("Public Risk", a.SamplePublicRisk, 15)
			drawSection("Samples", s.String())
		}

		// Footer
		pdf.SetY(-15)
		pdf.SetFont("Arial", "I", 8)
		pdf.SetTextColor(128, 128, 128)
		pdf.CellFormat(0, 10, tr(fmt.Sprintf("S3 Lifecycle Audit | %s", time.Now().Format("2006-01-02"))), "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 10, tr(fmt.Sprintf("Page %d", i+1)), "", 0, "R", false, 0, "")
	}

	if err := pdf.OutputFileAndClose(outputFilename); err != nil {
		return "", fmt.Errorf("error writing S3 lifecycle audit PDF: %w", err)
	}
	return filepath.Abs(outputFilename)
}

// ExportCommitmentsReportToCSV exporta o relatório de SP/RI para CSV, com aviso de "Data Unavailable".
func (r *ExportRepositoryImpl) ExportCommitmentsReportToCSV(reports []entity.CommitmentsReport, filename, outputDir string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "csv")
	if err != nil {
		return "", err
	}
	f, err := os.Create(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error creating commitments CSV file: %w", err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()

	headers := []string{
		"Profile", "Account ID", "Period",
		"SP Coverage %", "SP Util %", "SP Unused ($)",
		"RI Coverage %", "RI Util %", "RI Unused (hrs)",
		"Top SP (Service | Coverage% | OnDemand$)",
		"Top RI (Family | Coverage% | OnDemandHrs)",
	}
	if err := w.Write(headers); err != nil {
		return "", fmt.Errorf("error writing CSV header: %w", err)
	}

	for _, rep := range reports {
		period := fmt.Sprintf("%s to %s", rep.SPSummary.PeriodStart.Format("2006-01-02"), rep.SPSummary.PeriodEnd.Format("2006-01-02"))

		spCoverage := fmt.Sprintf("%.2f", rep.SPSummary.CoveragePercent)
		spUtil := fmt.Sprintf("%.2f", rep.SPSummary.UtilizationPercent)
		spUnused := fmt.Sprintf("%.2f", rep.SPSummary.UnusedCommitment)
		if rep.SPSummary.DataUnavailable {
			spCoverage, spUtil, spUnused = "Data Unavailable", "Data Unavailable", "Data Unavailable"
		}

		riCoverage := fmt.Sprintf("%.2f", rep.RISummary.CoveragePercent)
		riUtil := fmt.Sprintf("%.2f", rep.RISummary.UtilizationPercent)
		riUnused := fmt.Sprintf("%.2f", rep.RISummary.UnusedHours)
		if rep.RISummary.DataUnavailable {
			riCoverage, riUtil, riUnused = "Data Unavailable", "Data Unavailable", "Data Unavailable"
		}

		topSP := func(list []entity.ServiceCoverage, n int) string {
			if len(list) == 0 {
				return "N/A"
			}
			limit := len(list)
			if limit > n {
				limit = n
			}
			var lines []string
			for i := 0; i < limit; i++ {
				l := list[i]
				lines = append(lines, fmt.Sprintf("%s | %.2f%% | $%.2f", l.Service, l.CoveragePercent, l.OnDemandCost))
			}
			if len(list) > limit {
				lines = append(lines, fmt.Sprintf("... (+%d more)", len(list)-limit))
			}
			return cleanRichTags(strings.Join(lines, "\n"))
		}

		topRI := func(list []entity.ServiceCoverage, n int) string {
			if len(list) == 0 {
				return "N/A"
			}
			limit := len(list)
			if limit > n {
				limit = n
			}
			var lines []string
			for i := 0; i < limit; i++ {
				l := list[i]
				// OnDemandCost armazena horas no caso de RI
				lines = append(lines, fmt.Sprintf("%s | %.2f%% | %.2f hrs", l.Service, l.CoveragePercent, l.OnDemandCost))
			}
			if len(list) > limit {
				lines = append(lines, fmt.Sprintf("... (+%d more)", len(list)-limit))
			}
			return cleanRichTags(strings.Join(lines, "\n"))
		}

		record := []string{
			rep.Profile,
			rep.AccountID,
			period,
			spCoverage,
			spUtil,
			spUnused,
			riCoverage,
			riUtil,
			riUnused,
			topSP(rep.SPSummary.PerServiceCoverage, 5),
			topRI(rep.RISummary.PerServiceCoverage, 5),
		}
		if err := w.Write(record); err != nil {
			return "", fmt.Errorf("error writing CSV record: %w", err)
		}
	}
	return filepath.Abs(outputFilename)
}

func (r *ExportRepositoryImpl) ExportCommitmentsReportToJSON(reports []entity.CommitmentsReport, filename, outputDir string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "json")
	if err != nil {
		return "", err
	}
	f, err := os.Create(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error creating commitments JSON file: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(reports); err != nil {
		return "", fmt.Errorf("error encoding commitments JSON: %w", err)
	}
	return filepath.Abs(outputFilename)
}

// ExportCommitmentsReportToPDF exporta o relatório de SP/RI para PDF, com aviso de "Data Unavailable".
func (r *ExportRepositoryImpl) ExportCommitmentsReportToPDF(reports []entity.CommitmentsReport, filename, outputDir string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "pdf")
	if err != nil {
		return "", err
	}
	pdf := gofpdf.New("P", "mm", "A4", "")
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	for i, rep := range reports {
		pdf.AddPage()
		headerColor := [3]int{34, 139, 34}
		headerTextColor := [3]int{255, 255, 255}
		sectionTitleColor := [3]int{0, 0, 0}
		bodyTextColor := [3]int{50, 50, 50}
		lineColor := [3]int{200, 200, 200}

		drawSection := func(title string, content string) {
			content = cleanRichTags(content)
			if strings.TrimSpace(content) == "" {
				return
			}
			pdf.SetFont("Arial", "B", 12)
			pdf.SetTextColor(sectionTitleColor[0], sectionTitleColor[1], sectionTitleColor[2])
			pdf.Cell(0, 8, tr(title))
			pdf.Ln(7)
			pdf.SetDrawColor(lineColor[0], lineColor[1], lineColor[2])
			pdf.Line(pdf.GetX(), pdf.GetY(), pdf.GetX()+190, pdf.GetY())
			pdf.Ln(4)
			pdf.SetFont("Arial", "", 10)
			pdf.SetTextColor(bodyTextColor[0], bodyTextColor[1], bodyTextColor[2])
			pdf.MultiCell(190, 5, tr(content), "", "L", false)
			pdf.Ln(8)
		}

		// Header
		pdf.SetFillColor(headerColor[0], headerColor[1], headerColor[2])
		pdf.SetTextColor(headerTextColor[0], headerTextColor[1], headerTextColor[2])
		pdf.SetFont("Arial", "B", 14)
		pdf.CellFormat(0, 12, tr("  Savings Plans / RI Commitments"), "", 1, "L", true, 0, "")
		pdf.SetFont("Arial", "", 10)
		pdf.SetFillColor(240, 240, 240)
		pdf.SetTextColor(bodyTextColor[0], bodyTextColor[1], bodyTextColor[2])
		period := fmt.Sprintf("%s to %s", rep.SPSummary.PeriodStart.Format("2006-01-02"), rep.SPSummary.PeriodEnd.Format("2006-01-02"))
		pdf.CellFormat(0, 8, tr(fmt.Sprintf("  Profile: %s  |  Account ID: %s  |  Period: %s", rep.Profile, rep.AccountID, period)), "", 1, "L", true, 0, "")
		pdf.Ln(6)

		// SP Summary
		var spSummary string
		if rep.SPSummary.DataUnavailable {
			spSummary = "Data Unavailable"
		} else {
			spSummary = fmt.Sprintf("Coverage: %.2f%%\nUtilization: %.2f%%\nUnused Commitment: $%.2f",
				rep.SPSummary.CoveragePercent, rep.SPSummary.UtilizationPercent, rep.SPSummary.UnusedCommitment,
			)
		}
		drawSection("Savings Plans — Summary", spSummary)

		// SP Top Services
		if !rep.SPSummary.DataUnavailable && len(rep.SPSummary.PerServiceCoverage) > 0 {
			var b strings.Builder
			limit := len(rep.SPSummary.PerServiceCoverage)
			if limit > 12 {
				limit = 12
			}
			for j := 0; j < limit; j++ {
				l := rep.SPSummary.PerServiceCoverage[j]
				b.WriteString(fmt.Sprintf("%s: coverage %.2f%%, OnDemand $%.2f\n", l.Service, l.CoveragePercent, l.OnDemandCost))
			}
			if len(rep.SPSummary.PerServiceCoverage) > limit {
				b.WriteString(fmt.Sprintf("... (+%d more)\n", len(rep.SPSummary.PerServiceCoverage)-limit))
			}
			drawSection("Savings Plans — Top Services (by On-Demand $)", b.String())
		}

		// RI Summary
		var riSummary string
		if rep.RISummary.DataUnavailable {
			riSummary = "Data Unavailable"
		} else {
			riSummary = fmt.Sprintf("Coverage: %.2f%%\nUtilization: %.2f%%\nUnused Hours: %.2f\nUsed Hours: %.2f",
				rep.RISummary.CoveragePercent, rep.RISummary.UtilizationPercent, rep.RISummary.UnusedHours, rep.RISummary.UsedHours,
			)
		}
		drawSection("Reserved Instances — Summary", riSummary)

		// RI Top Services (ordenados por On-Demand Hours)
		if !rep.RISummary.DataUnavailable && len(rep.RISummary.PerServiceCoverage) > 0 {
			var b strings.Builder
			limit := len(rep.RISummary.PerServiceCoverage)
			if limit > 12 {
				limit = 12
			}
			for j := 0; j < limit; j++ {
				l := rep.RISummary.PerServiceCoverage[j]
				b.WriteString(fmt.Sprintf("%s: coverage %.2f%%, OnDemand Hrs %.2f\n", l.Service, l.CoveragePercent, l.OnDemandCost))
			}
			if len(rep.RISummary.PerServiceCoverage) > limit {
				b.WriteString(fmt.Sprintf("... (+%d more)\n", len(rep.RISummary.PerServiceCoverage)-limit))
			}
			drawSection("Reserved Instances — Top Families (by On-Demand Hours)", b.String())
		}

		// Footer
		pdf.SetY(-15)
		pdf.SetFont("Arial", "I", 8)
		pdf.SetTextColor(128, 128, 128)
		pdf.CellFormat(0, 10, tr(fmt.Sprintf("Commitments Report | %s", time.Now().Format("2006-01-02"))), "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 10, tr(fmt.Sprintf("Page %d", i+1)), "", 0, "R", false, 0, "")
	}

	if err := pdf.OutputFileAndClose(outputFilename); err != nil {
		return "", fmt.Errorf("error writing commitments PDF: %w", err)
	}
	return filepath.Abs(outputFilename)
}

// ExportFullAuditReportToCSV gera um pacote de arquivos CSV, um para cada sub-relatório.
func (r *ExportRepositoryImpl) ExportFullAuditReportToCSV(reports []entity.FullAuditReport, baseFilename, outputDir string) ([]string, error) {
	var generatedFiles []string

	// Extrai os sub-relatórios
	mainAudits := make([]entity.AuditData, 0, len(reports))
	transferAudits := make([]entity.DataTransferReport, 0, len(reports))
	logsAudits := make([]entity.CloudWatchLogsAudit, 0, len(reports))
	s3Audits := make([]entity.S3LifecycleAudit, 0, len(reports))
	commitmentsAudits := make([]entity.CommitmentsReport, 0, len(reports))

	for _, rep := range reports {
		if rep.MainAudit != nil {
			mainAudits = append(mainAudits, *rep.MainAudit)
		}
		if rep.TransferAudit != nil {
			transferAudits = append(transferAudits, *rep.TransferAudit)
		}
		if rep.LogsAudit != nil {
			logsAudits = append(logsAudits, *rep.LogsAudit)
		}
		if rep.S3Audit != nil {
			s3Audits = append(s3Audits, *rep.S3Audit)
		}
		if rep.CommitmentsAudit != nil {
			commitmentsAudits = append(commitmentsAudits, *rep.CommitmentsAudit)
		}
	}

	// Chama os exportadores individuais com nomes de arquivo derivados
	if len(mainAudits) > 0 {
		if path, err := r.ExportAuditReportToCSV(mainAudits, baseFilename+"_main", outputDir); err == nil {
			generatedFiles = append(generatedFiles, path)
		}
	}
	if len(transferAudits) > 0 {
		if path, err := r.ExportTransferReportToCSV(transferAudits, baseFilename+"_transfer", outputDir); err == nil {
			generatedFiles = append(generatedFiles, path)
		}
	}
	if len(logsAudits) > 0 {
		if path, err := r.ExportLogsAuditToCSV(logsAudits, baseFilename+"_logs", outputDir); err == nil {
			generatedFiles = append(generatedFiles, path)
		}
	}
	if len(s3Audits) > 0 {
		if path, err := r.ExportS3LifecycleAuditToCSV(s3Audits, baseFilename+"_s3", outputDir); err == nil {
			generatedFiles = append(generatedFiles, path)
		}
	}
	if len(commitmentsAudits) > 0 {
		if path, err := r.ExportCommitmentsReportToCSV(commitmentsAudits, baseFilename+"_commitments", outputDir); err == nil {
			generatedFiles = append(generatedFiles, path)
		}
	}

	return generatedFiles, nil
}

// ExportFullAuditReportToJSON gera um único JSON com todos os dados.
func (r *ExportRepositoryImpl) ExportFullAuditReportToJSON(reports []entity.FullAuditReport, filename, outputDir string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "json")
	if err != nil {
		return "", err
	}
	f, err := os.Create(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error creating full audit JSON file: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(reports); err != nil {
		return "", fmt.Errorf("error encoding full audit JSON: %w", err)
	}
	return filepath.Abs(outputFilename)
}

// ExportFullAuditReportToPDF gera um único PDF com "capítulos" para cada auditoria.
func (r *ExportRepositoryImpl) ExportFullAuditReportToPDF(reports []entity.FullAuditReport, filename, outputDir string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "pdf")
	if err != nil {
		return "", err
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	for _, rep := range reports {
		// --- Página de Rosto do Relatório para o Perfil ---
		pdf.AddPage()
		pdf.SetFont("Arial", "B", 24)
		pdf.SetTextColor(0, 0, 0)
		pdf.Cell(0, 20, "Full FinOps Audit Report")
		pdf.Ln(15)
		pdf.SetFont("Arial", "", 14)
		pdf.Cell(0, 10, fmt.Sprintf("Profile: %s", rep.Profile))
		pdf.Ln(8)
		pdf.Cell(0, 10, fmt.Sprintf("Account ID: %s", rep.AccountID))
		pdf.Ln(8)
		pdf.Cell(0, 10, fmt.Sprintf("Generated on: %s", time.Now().Format("2006-01-02 15:04:05")))
		pdf.Ln(20)

		// Índice
		pdf.SetFont("Arial", "B", 16)
		pdf.Cell(0, 10, "Table of Contents")
		pdf.Ln(10)
		pdf.SetFont("Arial", "", 12)
		if rep.MainAudit != nil {
			pdf.Cell(0, 8, "1. Main Audit (Unused, Untagged, etc.)")
			pdf.Ln(6)
		}
		if rep.TransferAudit != nil {
			pdf.Cell(0, 8, "2. Data Transfer Deep Dive")
			pdf.Ln(6)
		}
		if rep.LogsAudit != nil {
			pdf.Cell(0, 8, "3. CloudWatch Logs Retention")
			pdf.Ln(6)
		}
		if rep.S3Audit != nil {
			pdf.Cell(0, 8, "4. S3 Lifecycle & Security")
			pdf.Ln(6)
		}
		if rep.CommitmentsAudit != nil {
			pdf.Cell(0, 8, "5. Commitments (SP/RI)")
			pdf.Ln(6)
		}

		// --- Seções/Capítulos ---
		drawChapter := func(title string, drawContent func()) {
			pdf.AddPage()
			pdf.SetFont("Arial", "B", 18)
			pdf.SetFillColor(230, 230, 230)
			pdf.CellFormat(0, 12, fmt.Sprintf("  %s", title), "", 1, "L", true, 0, "")
			pdf.Ln(8)
			drawContent()
		}

		drawSection := func(title string, content string) {
			content = cleanRichTags(content)
			if strings.TrimSpace(content) == "" || content == "None" {
				return
			}
			pdf.SetFont("Arial", "B", 12)
			pdf.Cell(0, 8, tr(title))
			pdf.Ln(7)
			pdf.SetDrawColor(200, 200, 200)
			pdf.Line(pdf.GetX(), pdf.GetY(), pdf.GetX()+190, pdf.GetY())
			pdf.Ln(4)
			pdf.SetFont("Arial", "", 10)
			pdf.MultiCell(190, 5, tr(content), "", "L", false)
			pdf.Ln(8)
		}

		// 1. Main Audit
		if a := rep.MainAudit; a != nil {
			drawChapter("1. Main Audit", func() {
				drawSection("Budget Alerts", a.BudgetAlerts)
				drawSection("High-Cost NAT Gateways", a.NatGatewayCosts)
				drawSection("Unused VPC Endpoints", a.UnusedVpcEndpoints)
				drawSection("Idle Load Balancers", a.IdleLoadBalancers)
				drawSection("Stopped EC2 Instances", a.StoppedInstances)
				drawSection("Unused EBS Volumes", a.UnusedVolumes)
				drawSection("Unused Elastic IPs", a.UnusedEIPs)
				drawSection("Untagged Resources", a.UntaggedResources)
			})
		}

		// 2. Data Transfer
		if t := rep.TransferAudit; t != nil {
			drawChapter("2. Data Transfer Deep Dive", func() {
				var b strings.Builder
				b.WriteString(fmt.Sprintf("Total: $%.2f\n\n", t.Total))
				for _, c := range t.Categories {
					b.WriteString(fmt.Sprintf("%s: $%.2f\n", c.Category, c.Cost))
				}
				drawSection("Category Summary", b.String())

				if len(t.TopLines) > 0 {
					var tl strings.Builder
					for _, l := range t.TopLines {
						tl.WriteString(fmt.Sprintf("%s | %s: $%.2f\n", l.Service, l.UsageType, l.Cost))
					}
					drawSection("Top Lines", tl.String())
				}
			})
		}

		// 3. Logs Audit
		if l := rep.LogsAudit; l != nil {
			drawChapter("3. CloudWatch Logs Retention", func() {
				summary := fmt.Sprintf("No Retention (count): %d\nTotal Stored (GB): %.2f\n\nRecommendation: %s", l.NoRetentionCount, l.TotalStoredGB, l.RecommendedMessage)
				drawSection("Summary", summary)
				if len(l.NoRetentionTopN) > 0 {
					var b strings.Builder
					for _, lg := range l.NoRetentionTopN {
						b.WriteString(fmt.Sprintf("%s | %s | %.2f GB\n", lg.Region, lg.GroupName, float64(lg.StoredBytes)/(1024*1024*1024)))
					}
					drawSection("Top No-Retention Log Groups", b.String())
				}
			})
		}

		// 4. S3 Audit
		if s := rep.S3Audit; s != nil {
			drawChapter("4. S3 Lifecycle & Security", func() {
				summary := fmt.Sprintf("Total Buckets: %d\nNo Lifecycle: %d\nVersioned w/o Noncurrent Rule: %d\nNo Intelligent-Tiering: %d\nNo Default Encryption: %d\nPublic Risk: %d\n\nRecommendation: %s", s.TotalBuckets, s.NoLifecycleCount, s.VersionedWithoutNoncurrentLifecycle, s.NoIntelligentTieringCount, s.NoDefaultEncryptionCount, s.PublicRiskCount, s.RecommendedMessage)
				drawSection("Summary", summary)
				// Adicionar amostras se necessário
			})
		}

		// 5. Commitments
		if c := rep.CommitmentsAudit; c != nil {
			drawChapter("5. Commitments (SP/RI)", func() {
				// SP
				var spSummary string
				if c.SPSummary.DataUnavailable {
					spSummary = "Data Unavailable"
				} else {
					spSummary = fmt.Sprintf("Coverage: %.2f%%\nUtilization: %.2f%%\nUnused Commitment: $%.2f", c.SPSummary.CoveragePercent, c.SPSummary.UtilizationPercent, c.SPSummary.UnusedCommitment)
				}
				drawSection("Savings Plans — Summary", spSummary)
				if !c.SPSummary.DataUnavailable && len(c.SPSummary.PerServiceCoverage) > 0 {
					var b strings.Builder
					for _, l := range c.SPSummary.PerServiceCoverage {
						b.WriteString(fmt.Sprintf("%s: coverage %.2f%%, OnDemand $%.2f\n", l.Service, l.CoveragePercent, l.OnDemandCost))
					}
					drawSection("Savings Plans — Top Services (by On-Demand $)", b.String())
				}

				// RI
				var riSummary string
				if c.RISummary.DataUnavailable {
					riSummary = "Data Unavailable"
				} else {
					riSummary = fmt.Sprintf("Coverage: %.2f%%\nUtilization: %.2f%%\nUnused Hours: %.2f", c.RISummary.CoveragePercent, c.RISummary.UtilizationPercent, c.RISummary.UnusedHours)
				}
				drawSection("Reserved Instances — Summary", riSummary)
				if !c.RISummary.DataUnavailable && len(c.RISummary.PerServiceCoverage) > 0 {
					var b strings.Builder
					for _, l := range c.RISummary.PerServiceCoverage {
						b.WriteString(fmt.Sprintf("%s: coverage %.2f%%, OnDemand Hrs %.2f\n", l.Service, l.CoveragePercent, l.OnDemandCost))
					}
					drawSection("Reserved Instances — Top Families (by On-Demand Hours)", b.String())
				}
			})
		}
	}

	if err := pdf.OutputFileAndClose(outputFilename); err != nil {
		return "", fmt.Errorf("error writing full audit PDF file: %w", err)
	}
	return filepath.Abs(outputFilename)
}

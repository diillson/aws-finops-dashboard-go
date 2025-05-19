package export

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

// ExportToCSV exporta dados para um arquivo CSV.
func (r *ExportRepositoryImpl) ExportToCSV(
	data []entity.ProfileData,
	filename string,
	outputDir string,
	previousPeriodDates string,
	currentPeriodDates string,
) (string, error) {
	timestamp := time.Now().Format("20060102_1504")
	baseFilename := fmt.Sprintf("%s_%s.csv", filename, timestamp)

	// Garantir que o diretório de saída existe
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("error creating output directory: %w", err)
	}

	outputFilename := filepath.Join(outputDir, baseFilename)

	file, err := os.Create(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error creating CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	previousPeriodHeader := fmt.Sprintf("Cost for period\n(%s)", previousPeriodDates)
	currentPeriodHeader := fmt.Sprintf("Cost for period\n(%s)", currentPeriodDates)

	// Escreve o cabeçalho
	headers := []string{
		"CLI Profile",
		"AWS Account ID",
		previousPeriodHeader,
		currentPeriodHeader,
		"Cost By Service",
		"Budget Status",
		"EC2 Instances",
	}
	if err := writer.Write(headers); err != nil {
		return "", fmt.Errorf("error writing CSV header: %w", err)
	}

	// Escreve os dados
	for _, row := range data {
		servicesData := ""
		for _, sc := range row.ServiceCosts {
			servicesData += fmt.Sprintf("%s: $%.2f\n", sc.ServiceName, sc.Cost)
		}

		budgetsData := strings.Join(row.BudgetInfo, "\n")
		if budgetsData == "" {
			budgetsData = "No budgets"
		}

		ec2Data := ""
		for state, count := range row.EC2Summary {
			if count > 0 {
				ec2Data += fmt.Sprintf("%s: %d\n", state, count)
			}
		}
		if ec2Data == "" {
			ec2Data = "No instances"
		}

		record := []string{
			row.Profile,
			row.AccountID,
			fmt.Sprintf("$%.2f", row.LastMonth),
			fmt.Sprintf("$%.2f", row.CurrentMonth),
			servicesData,
			budgetsData,
			ec2Data,
		}

		if err := writer.Write(record); err != nil {
			return "", fmt.Errorf("error writing CSV row: %w", err)
		}
	}

	absPath, err := filepath.Abs(outputFilename)
	if err != nil {
		return outputFilename, nil
	}
	return absPath, nil
}

// ExportToJSON exporta dados para um arquivo JSON.
func (r *ExportRepositoryImpl) ExportToJSON(
	data []entity.ProfileData,
	filename string,
	outputDir string,
) (string, error) {
	timestamp := time.Now().Format("20060102_1504")
	baseFilename := fmt.Sprintf("%s_%s.json", filename, timestamp)

	// Garantir que o diretório de saída existe
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("error creating output directory: %w", err)
	}

	outputFilename := filepath.Join(outputDir, baseFilename)

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

	absPath, err := filepath.Abs(outputFilename)
	if err != nil {
		return outputFilename, nil
	}
	return absPath, nil
}

// ExportToPDF exporta dados para um arquivo PDF.
func (r *ExportRepositoryImpl) ExportToPDF(
	data []entity.ProfileData,
	filename string,
	outputDir string,
	previousPeriodDates string,
	currentPeriodDates string,
) (string, error) {
	timestamp := time.Now().Format("20060102_1504")
	baseFilename := fmt.Sprintf("%s_%s.pdf", filename, timestamp)

	// Garantir que o diretório de saída existe
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("error creating output directory: %w", err)
	}

	outputFilename := filepath.Join(outputDir, baseFilename)

	pdf := gofpdf.New("L", "mm", "Letter", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)
	pdf.Cell(40, 10, "AWS FinOps Dashboard (Cost Report)")
	pdf.Ln(15)

	// Define a tabela
	headers := []string{
		"CLI Profile",
		"AWS Account ID",
		fmt.Sprintf("Cost for period\n(%s)", previousPeriodDates),
		fmt.Sprintf("Cost for period\n(%s)", currentPeriodDates),
		"Cost By Service",
		"Budget Status",
		"EC2 Instances",
	}

	// Calcula a largura de cada coluna
	tableWidth := 260.0 // ajuste conforme necessário
	colWidth := tableWidth / float64(len(headers))

	// Cabeçalho
	pdf.SetFont("Arial", "B", 10)
	pdf.SetFillColor(0, 0, 0)
	pdf.SetTextColor(255, 255, 255)
	for _, header := range headers {
		pdf.CellFormat(colWidth, 10, header, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)

	// Dados
	pdf.SetFont("Arial", "", 8)
	pdf.SetTextColor(0, 0, 0)
	fill := false
	for _, row := range data {
		pdf.SetFillColor(240, 240, 240)
		fill = !fill

		// Perfil
		pdf.CellFormat(colWidth, 10, row.Profile, "1", 0, "L", fill, 0, "")

		// ID da conta
		pdf.CellFormat(colWidth, 10, row.AccountID, "1", 0, "L", fill, 0, "")

		// Custo mês anterior
		pdf.CellFormat(colWidth, 10, fmt.Sprintf("$%.2f", row.LastMonth), "1", 0, "R", fill, 0, "")

		// Custo mês atual
		pdf.CellFormat(colWidth, 10, fmt.Sprintf("$%.2f", row.CurrentMonth), "1", 0, "R", fill, 0, "")

		// Custos por serviço
		serviceCosts := ""
		for _, sc := range row.ServiceCosts {
			serviceCosts += fmt.Sprintf("%s: $%.2f\n", sc.ServiceName, sc.Cost)
		}
		pdf.CellFormat(colWidth, 10, serviceCosts, "1", 0, "L", fill, 0, "")

		// Status do orçamento
		budgetInfo := strings.Join(row.BudgetInfo, "\n")
		pdf.CellFormat(colWidth, 10, budgetInfo, "1", 0, "L", fill, 0, "")

		// Resumo EC2
		ec2Summary := ""
		for state, count := range row.EC2Summary {
			if count > 0 {
				ec2Summary += fmt.Sprintf("%s: %d\n", state, count)
			}
		}
		pdf.CellFormat(colWidth, 10, ec2Summary, "1", 0, "L", fill, 0, "")

		pdf.Ln(-1)
	}

	// Adiciona rodapé com timestamp
	currentTime := time.Now().Format("2006-01-02 15:04:05")
	footerText := fmt.Sprintf("This report is generated using AWS FinOps Dashboard (CLI) © 2023 on %s", currentTime)
	pdf.SetY(-15)
	pdf.SetFont("Arial", "I", 8)
	pdf.Cell(0, 10, footerText)

	// Salva o PDF
	err := pdf.OutputFileAndClose(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error writing PDF file: %w", err)
	}

	absPath, err := filepath.Abs(outputFilename)
	if err != nil {
		return outputFilename, nil
	}
	return absPath, nil
}

// ExportAuditReportToPDF exporta um relatório de auditoria para um arquivo PDF.
func (r *ExportRepositoryImpl) ExportAuditReportToPDF(
	auditData []entity.AuditData,
	filename string,
	outputDir string,
) (string, error) {
	timestamp := time.Now().Format("20060102_1504")
	baseFilename := fmt.Sprintf("%s_%s.pdf", filename, timestamp)

	// Garantir que o diretório de saída existe
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("error creating output directory: %w", err)
	}

	outputFilename := filepath.Join(outputDir, baseFilename)

	pdf := gofpdf.New("L", "mm", "Letter", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)
	pdf.Cell(40, 10, "AWS FinOps Dashboard (Audit Report)")
	pdf.Ln(15)

	// Define a tabela
	headers := []string{
		"Profile",
		"Account ID",
		"Untagged Resources",
		"Stopped EC2 Instances",
		"Unused Volumes",
		"Unused EIPs",
		"Budget Alerts",
	}

	// Calcula a largura de cada coluna
	tableWidth := 260.0 // ajuste conforme necessário
	colWidth := tableWidth / float64(len(headers))

	// Cabeçalho
	pdf.SetFont("Arial", "B", 10)
	pdf.SetFillColor(0, 0, 0)
	pdf.SetTextColor(255, 255, 255)
	for _, header := range headers {
		pdf.CellFormat(colWidth, 10, header, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)

	// Dados
	pdf.SetFont("Arial", "", 8)
	pdf.SetTextColor(0, 0, 0)
	fill := false
	for _, row := range auditData {
		pdf.SetFillColor(240, 240, 240)
		fill = !fill

		// Remove as tags Rich da versão Python
		untaggedResources := cleanRichTags(row.UntaggedResources)
		stoppedInstances := cleanRichTags(row.StoppedInstances)
		unusedVolumes := cleanRichTags(row.UnusedVolumes)
		unusedEIPs := cleanRichTags(row.UnusedEIPs)
		budgetAlerts := cleanRichTags(row.BudgetAlerts)

		pdf.CellFormat(colWidth, 10, row.Profile, "1", 0, "L", fill, 0, "")
		pdf.CellFormat(colWidth, 10, row.AccountID, "1", 0, "L", fill, 0, "")
		pdf.CellFormat(colWidth, 10, untaggedResources, "1", 0, "L", fill, 0, "")
		pdf.CellFormat(colWidth, 10, stoppedInstances, "1", 0, "L", fill, 0, "")
		pdf.CellFormat(colWidth, 10, unusedVolumes, "1", 0, "L", fill, 0, "")
		pdf.CellFormat(colWidth, 10, unusedEIPs, "1", 0, "L", fill, 0, "")
		pdf.CellFormat(colWidth, 10, budgetAlerts, "1", 0, "L", fill, 0, "")

		pdf.Ln(-1)
	}

	// Adiciona rodapé com informações
	pdf.Ln(10)
	pdf.SetFont("Arial", "I", 8)
	pdf.Cell(0, 10, "Note: This table lists untagged EC2, RDS, Lambda, ELBv2 only.")

	// Adiciona rodapé com timestamp
	currentTime := time.Now().Format("2006-01-02 15:04:05")
	footerText := fmt.Sprintf("This audit report is generated using AWS FinOps Dashboard (CLI) © 2023 on %s", currentTime)
	pdf.SetY(-15)
	pdf.SetFont("Arial", "I", 8)
	pdf.Cell(0, 10, footerText)

	// Salva o PDF
	err := pdf.OutputFileAndClose(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error writing PDF file: %w", err)
	}

	absPath, err := filepath.Abs(outputFilename)
	if err != nil {
		return outputFilename, nil
	}
	return absPath, nil
}

// ExportAuditReportToCSV exporta um relatório de auditoria para um arquivo CSV.
func (r *ExportRepositoryImpl) ExportAuditReportToCSV(
	auditData []entity.AuditData,
	filename string,
	outputDir string,
) (string, error) {
	timestamp := time.Now().Format("20060102_1504")
	baseFilename := fmt.Sprintf("%s_%s.csv", filename, timestamp)

	// Garantir que o diretório de saída existe
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("error creating output directory: %w", err)
	}

	outputFilename := filepath.Join(outputDir, baseFilename)

	file, err := os.Create(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error creating CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Escreve o cabeçalho
	headers := []string{
		"Profile",
		"Account ID",
		"Untagged Resources",
		"Stopped EC2 Instances",
		"Unused Volumes",
		"Unused EIPs",
		"Budget Alerts",
	}
	if err := writer.Write(headers); err != nil {
		return "", fmt.Errorf("error writing CSV header: %w", err)
	}

	// Escreve os dados
	for _, row := range auditData {
		// Remove as tags Rich para uma melhor visualização no CSV
		untaggedResources := cleanRichTags(row.UntaggedResources)
		stoppedInstances := cleanRichTags(row.StoppedInstances)
		unusedVolumes := cleanRichTags(row.UnusedVolumes)
		unusedEIPs := cleanRichTags(row.UnusedEIPs)
		budgetAlerts := cleanRichTags(row.BudgetAlerts)

		record := []string{
			row.Profile,
			row.AccountID,
			untaggedResources,
			stoppedInstances,
			unusedVolumes,
			unusedEIPs,
			budgetAlerts,
		}

		if err := writer.Write(record); err != nil {
			return "", fmt.Errorf("error writing CSV row: %w", err)
		}
	}

	absPath, err := filepath.Abs(outputFilename)
	if err != nil {
		return outputFilename, nil
	}
	return absPath, nil
}

// ExportAuditReportToJSON exporta um relatório de auditoria para um arquivo JSON.
func (r *ExportRepositoryImpl) ExportAuditReportToJSON(
	auditData []entity.AuditData,
	filename string,
	outputDir string,
) (string, error) {
	timestamp := time.Now().Format("20060102_1504")
	baseFilename := fmt.Sprintf("%s_%s.json", filename, timestamp)

	// Garantir que o diretório de saída existe
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("error creating output directory: %w", err)
	}

	outputFilename := filepath.Join(outputDir, baseFilename)

	file, err := os.Create(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error creating JSON file: %w", err)
	}
	defer file.Close()

	// Cria cópias limpas para JSON
	cleanData := make([]entity.AuditData, len(auditData))
	for i, row := range auditData {
		cleanData[i] = entity.AuditData{
			Profile:           row.Profile,
			AccountID:         row.AccountID,
			UntaggedResources: cleanRichTags(row.UntaggedResources),
			StoppedInstances:  cleanRichTags(row.StoppedInstances),
			UnusedVolumes:     cleanRichTags(row.UnusedVolumes),
			UnusedEIPs:        cleanRichTags(row.UnusedEIPs),
			BudgetAlerts:      cleanRichTags(row.BudgetAlerts),
		}
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cleanData); err != nil {
		return "", fmt.Errorf("error encoding JSON data: %w", err)
	}

	absPath, err := filepath.Abs(outputFilename)
	if err != nil {
		return outputFilename, nil
	}
	return absPath, nil
}

// cleanRichTags remove as tags de estilo Rich texto para exportação.
func cleanRichTags(text string) string {
	// Regex para remover padrões como [bold red], [/], etc.
	re := regexp.MustCompile(`\[\/?(bold|bright_red|bright_green|bright_yellow|bright_cyan|bright_magenta|dark_magenta|dark_orange|gold1|orange1|red1|yellow|)\]`)
	return re.ReplaceAllString(text, "")
}

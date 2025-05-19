package usecase

import (
	"context"
	"fmt"
	"github.com/pterm/pterm"
	"math"
	"sort"
	"strings"

	"github.com/diillson/aws-finops-dashboard-go/internal/domain/entity"
	"github.com/diillson/aws-finops-dashboard-go/internal/domain/repository"
	"github.com/diillson/aws-finops-dashboard-go/internal/shared/types"
)

// DashboardUseCase handles the main dashboard functionality.
type DashboardUseCase struct {
	awsRepo    repository.AWSRepository
	exportRepo repository.ExportRepository
	configRepo repository.ConfigRepository
	console    types.ConsoleInterface
}

// NewDashboardUseCase creates a new dashboard use case.
func NewDashboardUseCase(
	awsRepo repository.AWSRepository,
	exportRepo repository.ExportRepository,
	configRepo repository.ConfigRepository,
	console types.ConsoleInterface,
) *DashboardUseCase {
	return &DashboardUseCase{
		awsRepo:    awsRepo,
		exportRepo: exportRepo,
		configRepo: configRepo,
		console:    console,
	}
}

// InitializeProfiles determines which AWS profiles to use based on CLI args.
func (uc *DashboardUseCase) InitializeProfiles(args *types.CLIArgs) ([]string, []string, int, error) {
	availableProfiles := uc.awsRepo.GetAWSProfiles()
	if len(availableProfiles) == 0 {
		return nil, nil, 0, types.ErrNoProfilesFound
	}

	profilesToUse := []string{}

	if len(args.Profiles) > 0 {
		for _, profile := range args.Profiles {
			found := false
			for _, availProfile := range availableProfiles {
				if profile == availProfile {
					profilesToUse = append(profilesToUse, profile)
					found = true
					break
				}
			}
			if !found {
				uc.console.LogWarning("Profile '%s' not found in AWS configuration", profile)
			}
		}
		if len(profilesToUse) == 0 {
			return nil, nil, 0, types.ErrNoValidProfilesFound
		}
	} else if args.All {
		profilesToUse = availableProfiles
	} else {
		// Check if default profile exists
		defaultExists := false
		for _, profile := range availableProfiles {
			if profile == "default" {
				profilesToUse = []string{"default"}
				defaultExists = true
				break
			}
		}

		if !defaultExists {
			profilesToUse = availableProfiles
			uc.console.LogWarning("No default profile found. Using all available profiles.")
		}
	}

	var timeRange int
	if args.TimeRange != nil {
		timeRange = *args.TimeRange
	}

	return profilesToUse, args.Regions, timeRange, nil
}

// ProcessSingleProfile implementa o processamento de um único perfil AWS.
func (uc *DashboardUseCase) ProcessSingleProfile(
	ctx context.Context,
	profile string,
	userRegions []string,
	timeRange int,
	tags []string,
) entity.ProfileData {
	var profileData entity.ProfileData
	profileData.Profile = profile
	profileData.Success = false

	// Obtém dados de custo
	costData, err := uc.awsRepo.GetCostData(ctx, profile, &timeRange, tags)
	if err != nil {
		profileData.Error = err.Error()
		return profileData
	}

	// Define regiões a serem usadas
	regions := userRegions
	if len(regions) == 0 {
		regions, err = uc.awsRepo.GetAccessibleRegions(ctx, profile)
		if err != nil {
			profileData.Error = err.Error()
			return profileData
		}
	}

	// Obtém resumo das instâncias EC2
	ec2Summary, err := uc.awsRepo.GetEC2Summary(ctx, profile, regions)
	if err != nil {
		profileData.Error = err.Error()
		return profileData
	}

	// Processa custos por serviço
	serviceCosts, serviceCostsFormatted := uc.processServiceCosts(costData)

	// Formata informações do orçamento
	budgetInfo := uc.formatBudgetInfo(costData.Budgets)

	// Formata resumo do EC2
	ec2SummaryFormatted := uc.formatEC2Summary(ec2Summary)

	// Calcula alteração percentual no custo total
	var percentChange *float64
	if costData.LastMonthCost > 0.01 {
		change := ((costData.CurrentMonthCost - costData.LastMonthCost) / costData.LastMonthCost) * 100.0
		percentChange = &change
	} else if costData.CurrentMonthCost < 0.01 {
		change := 0.0
		percentChange = &change
	}

	// Preenche o dado do perfil
	profileData = entity.ProfileData{
		Profile:               profile,
		AccountID:             costData.AccountID,
		LastMonth:             costData.LastMonthCost,
		CurrentMonth:          costData.CurrentMonthCost,
		ServiceCosts:          serviceCosts,
		ServiceCostsFormatted: serviceCostsFormatted,
		BudgetInfo:            budgetInfo,
		EC2Summary:            ec2Summary,
		EC2SummaryFormatted:   ec2SummaryFormatted,
		Success:               true,
		CurrentPeriodName:     costData.CurrentPeriodName,
		PreviousPeriodName:    costData.PreviousPeriodName,
		PercentChangeInCost:   percentChange,
	}

	return profileData
}

// RunDashboard executa a funcionalidade principal do dashboard.
func (uc *DashboardUseCase) RunDashboard(
	ctx context.Context,
	args *types.CLIArgs,
) error {
	// Inicializa os perfis com base nos argumentos da CLI
	profilesToUse, userRegions, timeRange, err := uc.InitializeProfiles(args)
	if err != nil {
		return err
	}

	// Executa relatório de auditoria se solicitado
	if args.Audit {
		auditData, err := uc.RunAuditReport(ctx, profilesToUse, args)
		if err != nil {
			return err
		}

		// Exporta o relatório de auditoria se um nome de relatório for fornecido
		if args.ReportName != "" {
			for _, reportType := range args.ReportType {
				switch reportType {
				case "pdf":
					pdfPath, err := uc.exportRepo.ExportAuditReportToPDF(auditData, args.ReportName, args.Dir)
					if err != nil {
						uc.console.LogError("Failed to export audit report to PDF: %s", err)
					} else {
						uc.console.LogSuccess("Successfully exported audit report to PDF: %s", pdfPath)
					}
				case "csv":
					csvPath, err := uc.exportRepo.ExportAuditReportToCSV(auditData, args.ReportName, args.Dir)
					if err != nil {
						uc.console.LogError("Failed to export audit report to CSV: %s", err)
					} else {
						uc.console.LogSuccess("Successfully exported audit report to CSV: %s", csvPath)
					}
				case "json":
					jsonPath, err := uc.exportRepo.ExportAuditReportToJSON(auditData, args.ReportName, args.Dir)
					if err != nil {
						uc.console.LogError("Failed to export audit report to JSON: %s", err)
					} else {
						uc.console.LogSuccess("Successfully exported audit report to JSON: %s", jsonPath)
					}
				}
			}
		}
		return nil
	}

	// Executa análise de tendência se solicitada
	if args.Trend {
		err := uc.RunTrendAnalysis(ctx, profilesToUse, args)
		if err != nil {
			return err
		}
		return nil
	}

	// Inicializa o dashboard principal
	status := uc.console.Status("Initializing dashboard...")

	// Obtém informações do período para a tabela de exibição
	previousPeriodName, currentPeriodName, previousPeriodDates, currentPeriodDates :=
		uc.getDisplayTablePeriodInfo(ctx, profilesToUse, timeRange)

	// Cria a tabela de exibição
	table := uc.createDisplayTable(previousPeriodDates, currentPeriodDates, previousPeriodName, currentPeriodName)

	//status.Stop()

	// Gera os dados do dashboard
	exportData, err := uc.generateDashboardData(ctx, profilesToUse, userRegions, timeRange, args, table, status)
	if err != nil {
		status.Stop()
		return err
	}

	status.Stop()

	// Exibe a tabela
	uc.console.Print(table.Render())

	// Exporta os relatórios do dashboard
	if args.ReportName != "" && len(args.ReportType) > 0 {
		for _, reportType := range args.ReportType {
			switch reportType {
			case "csv":
				csvPath, err := uc.exportRepo.ExportToCSV(exportData, args.ReportName, args.Dir, previousPeriodDates, currentPeriodDates)
				if err != nil {
					uc.console.LogError("Failed to export to CSV: %s", err)
				} else {
					uc.console.LogSuccess("Successfully exported to CSV: %s", csvPath)
				}
			case "json":
				jsonPath, err := uc.exportRepo.ExportToJSON(exportData, args.ReportName, args.Dir)
				if err != nil {
					uc.console.LogError("Failed to export to JSON: %s", err)
				} else {
					uc.console.LogSuccess("Successfully exported to JSON: %s", jsonPath)
				}
			case "pdf":
				pdfPath, err := uc.exportRepo.ExportToPDF(exportData, args.ReportName, args.Dir, previousPeriodDates, currentPeriodDates)
				if err != nil {
					uc.console.LogError("Failed to export to PDF: %s", err)
				} else {
					uc.console.LogSuccess("\nSuccessfully exported to PDF: %s", pdfPath)
				}
			}
		}
	}

	return nil
}

// RunAuditReport executa um relatório de auditoria para os perfis especificados.
func (uc *DashboardUseCase) RunAuditReport(
	ctx context.Context,
	profilesToUse []string,
	args *types.CLIArgs,
) ([]entity.AuditData, error) {
	uc.console.LogInfo("Preparing your audit report...")

	table := uc.console.CreateTable()
	table.AddColumn("Profile")
	table.AddColumn("Account ID")
	table.AddColumn("Untagged Resources")
	table.AddColumn("Stopped EC2 Instances")
	table.AddColumn("Unused Volumes")
	table.AddColumn("Unused EIPs")
	table.AddColumn("Budget Alerts")

	auditDataList := []entity.AuditData{}
	nl := "\n"

	for _, profile := range profilesToUse {
		_, err := uc.awsRepo.GetSession(ctx, profile)
		if err != nil {
			uc.console.LogError("Failed to create session for profile %s: %s", profile, err)
			continue
		}

		accountID, err := uc.awsRepo.GetAccountID(ctx, profile)
		if err != nil {
			accountID = "Unknown"
		}

		regions := args.Regions
		if len(regions) == 0 {
			regions, err = uc.awsRepo.GetAccessibleRegions(ctx, profile)
			if err != nil {
				uc.console.LogWarning("Could not get accessible regions for profile %s: %s", profile, err)
				regions = []string{"us-east-1", "us-west-2", "eu-west-1"} // defaults
			}
		}

		// Obtém recursos não marcados
		untagged, err := uc.awsRepo.GetUntaggedResources(ctx, profile, regions)
		var anomalies []string
		if err != nil {
			anomalies = []string{fmt.Sprintf("Error: %s", err)}
		} else {
			for service, regionMap := range untagged {
				if len(regionMap) > 0 {
					serviceBlock := fmt.Sprintf("%s:\n", pterm.FgYellow.Sprint(service))
					for region, ids := range regionMap {
						if len(ids) > 0 {
							idsBlock := strings.Join(ids, "\n")
							serviceBlock += fmt.Sprintf("\n%s:\n%s\n", region, idsBlock)
						}
					}
					anomalies = append(anomalies, serviceBlock)
				}
			}
			if len(anomalies) == 0 {
				anomalies = []string{"None"}
			}
		}

		// Obtém instâncias EC2 paradas
		stopped, err := uc.awsRepo.GetStoppedInstances(ctx, profile, regions)
		stoppedList := []string{}
		if err != nil {
			stoppedList = []string{fmt.Sprintf("Error: %s", err)}
		} else {
			for region, ids := range stopped {
				if len(ids) > 0 {
					stoppedList = append(stoppedList, fmt.Sprintf("%s:\n%s", region,
						pterm.NewStyle(pterm.FgYellow).Sprint(strings.Join(ids, nl))))
				}
			}
			if len(stoppedList) == 0 {
				stoppedList = []string{"None"}
			}
		}

		// Obtém volumes não utilizados
		unusedVols, err := uc.awsRepo.GetUnusedVolumes(ctx, profile, regions)
		volsList := []string{}
		if err != nil {
			volsList = []string{fmt.Sprintf("Error: %s", err)}
		} else {
			for region, ids := range unusedVols {
				if len(ids) > 0 {
					volsList = append(volsList, fmt.Sprintf("%s:\n%s", region,
						pterm.NewStyle(pterm.FgLightRed).Sprint(strings.Join(ids, nl))))
				}
			}
			if len(volsList) == 0 {
				volsList = []string{"None"}
			}
		}

		// Obtém EIPs não utilizados
		unusedEIPs, err := uc.awsRepo.GetUnusedEIPs(ctx, profile, regions)
		eipsList := []string{}
		if err != nil {
			eipsList = []string{fmt.Sprintf("Error: %s", err)}
		} else {
			for region, ids := range unusedEIPs {
				if len(ids) > 0 {
					eipsList = append(eipsList, fmt.Sprintf("%s:\n%s", region, strings.Join(ids, ",\n")))
				}
			}
			if len(eipsList) == 0 {
				eipsList = []string{"None"}
			}
		}

		// Obtém alertas de orçamento
		budgetData, err := uc.awsRepo.GetBudgets(ctx, profile)
		alerts := []string{}
		if err != nil {
			alerts = []string{fmt.Sprintf("Error: %s", err)}
		} else {
			for _, b := range budgetData {
				if b.Actual > b.Limit {
					alerts = append(alerts,
						fmt.Sprintf("%s: $%.2f > $%.2f", pterm.FgRed.Sprint(b.Name), b.Actual, b.Limit))
				}
			}
			if len(alerts) == 0 {
				alerts = []string{"No budgets exceeded"}
			}
		}

		auditData := entity.AuditData{
			Profile:           profile,
			AccountID:         accountID,
			UntaggedResources: strings.Join(anomalies, "\n"),
			StoppedInstances:  strings.Join(stoppedList, "\n"),
			UnusedVolumes:     strings.Join(volsList, "\n"),
			UnusedEIPs:        strings.Join(eipsList, "\n"),
			BudgetAlerts:      strings.Join(alerts, "\n"),
		}
		auditDataList = append(auditDataList, auditData)

		table.AddRow(
			pterm.FgMagenta.Sprintf("%s", profile),
			accountID,
			strings.Join(anomalies, "\n"),
			strings.Join(stoppedList, "\n"),
			strings.Join(volsList, "\n"),
			strings.Join(eipsList, "\n"),
			strings.Join(alerts, "\n"),
		)
	}

	uc.console.Print(table.Render())
	fmt.Println()
	uc.console.LogInfo("Note: The dashboard only lists untagged EC2, RDS, Lambda, ELBv2.\n")

	return auditDataList, nil
}

// RunTrendAnalysis executa uma análise de tendência de custo para os perfis especificados.
func (uc *DashboardUseCase) RunTrendAnalysis(
	ctx context.Context,
	profilesToUse []string,
	args *types.CLIArgs,
) error {
	uc.console.LogInfo("Analysing cost trends...")

	if args.Combine {
		accountProfiles := make(map[string][]string)

		for _, profile := range profilesToUse {
			accountID, err := uc.awsRepo.GetAccountID(ctx, profile)
			if err != nil {
				uc.console.LogError("Error checking account ID for profile %s: %s", profile, err)
				continue
			}

			accountProfiles[accountID] = append(accountProfiles[accountID], profile)
		}

		for accountID, profiles := range accountProfiles {
			primaryProfile := profiles[0]
			trendData, err := uc.awsRepo.GetTrendData(ctx, primaryProfile, args.Tag)
			if err != nil {
				uc.console.LogError("Error getting trend for account %s: %s", accountID, err)
				continue
			}

			monthlyCosts, ok := trendData["monthly_costs"].([]entity.MonthlyCost)
			if !ok || len(monthlyCosts) == 0 {
				uc.console.LogWarning("No trend data available for account %s", accountID)
				continue
			}

			// Converte para o tipo correto
			uiMonthlyCosts := make([]types.MonthlyCost, len(monthlyCosts))
			for i, mc := range monthlyCosts {
				uiMonthlyCosts[i] = types.MonthlyCost{
					Month: mc.Month,
					Cost:  mc.Cost,
				}
			}

			profileList := strings.Join(profiles, ", ")
			uc.console.Printf("\n%s\n",
				pterm.FgYellow.Sprintf("Account: %s (Profiles: %s)", accountID, profileList))
			uc.console.DisplayTrendBars(uiMonthlyCosts)
		}
	} else {
		for _, profile := range profilesToUse {
			trendData, err := uc.awsRepo.GetTrendData(ctx, profile, args.Tag)
			if err != nil {
				uc.console.LogError("Error getting trend for profile %s: %s", profile, err)
				continue
			}

			monthlyCosts, ok := trendData["monthly_costs"].([]entity.MonthlyCost)
			if !ok || len(monthlyCosts) == 0 {
				uc.console.LogWarning("No trend data available for profile %s", profile)
				continue
			}

			accountID, _ := trendData["account_id"].(string)
			if accountID == "" {
				accountID = "Unknown"
			}

			// Converte para o tipo correto
			uiMonthlyCosts := make([]types.MonthlyCost, len(monthlyCosts))
			for i, mc := range monthlyCosts {
				uiMonthlyCosts[i] = types.MonthlyCost{
					Month: mc.Month,
					Cost:  mc.Cost,
				}
			}

			uc.console.Printf("\n%s\n",
				pterm.FgYellow.Sprintf("Account: %s (Profile: %s)", accountID, profile))
			uc.console.DisplayTrendBars(uiMonthlyCosts)
		}
	}

	return nil
}

// Funções auxiliares para o DashboardUseCase

// processServiceCosts processa e formata os custos do serviço a partir dos dados de custo.
func (uc *DashboardUseCase) processServiceCosts(costData entity.CostData) ([]entity.ServiceCost, []string) {
	serviceCosts := []entity.ServiceCost{}
	serviceCostsFormatted := []string{}

	// Considerando que CostData.CurrentMonthCostByService já tem um slice de ServiceCost
	for _, serviceCost := range costData.CurrentMonthCostByService {
		if serviceCost.Cost > 0.001 {
			serviceCosts = append(serviceCosts, serviceCost)
		}
	}

	// Ordena os serviços por custo (em ordem decrescente)
	sort.Slice(serviceCosts, func(i, j int) bool {
		return serviceCosts[i].Cost > serviceCosts[j].Cost
	})

	if len(serviceCosts) == 0 {
		serviceCostsFormatted = append(serviceCostsFormatted, "No costs associated with this account")
	} else {
		for _, sc := range serviceCosts {
			serviceCostsFormatted = append(serviceCostsFormatted, fmt.Sprintf("%s: $%.2f", sc.ServiceName, sc.Cost))
		}
	}

	return serviceCosts, serviceCostsFormatted
}

// formatBudgetInfo formata as informações do orçamento para exibição.
func (uc *DashboardUseCase) formatBudgetInfo(budgets []entity.BudgetInfo) []string {
	budgetInfo := []string{}

	for _, budget := range budgets {
		budgetInfo = append(budgetInfo, fmt.Sprintf("%s limit: $%.2f", budget.Name, budget.Limit))
		budgetInfo = append(budgetInfo, fmt.Sprintf("%s actual: $%.2f", budget.Name, budget.Actual))
		if budget.Forecast > 0 {
			budgetInfo = append(budgetInfo, fmt.Sprintf("%s forecast: $%.2f", budget.Name, budget.Forecast))
		}
	}

	if len(budgetInfo) == 0 {
		budgetInfo = append(budgetInfo, "No budgets found;\nCreate a budget for this account")
	}

	return budgetInfo
}

// formatEC2Summary formata o resumo da instância EC2 para exibição.
func (uc *DashboardUseCase) formatEC2Summary(ec2Data entity.EC2Summary) []string {
	ec2SummaryText := []string{}

	for state, count := range ec2Data {
		if count > 0 {
			var stateText string
			if state == "running" {
				stateText = fmt.Sprintf("%s: %d", pterm.FgGreen.Sprint(state), count)
			} else if state == "stopped" {
				stateText = fmt.Sprintf("%s: %d", pterm.FgYellow.Sprint(state), count)
			} else {
				stateText = fmt.Sprintf("%s: %d", pterm.FgCyan.Sprint(state), count)
			}
			ec2SummaryText = append(ec2SummaryText, stateText)
		}
	}

	if len(ec2SummaryText) == 0 {
		ec2SummaryText = []string{"No instances found"}
	}

	return ec2SummaryText
}

// getDisplayTablePeriodInfo obtém informações do período para a tabela de exibição.
func (uc *DashboardUseCase) getDisplayTablePeriodInfo(
	ctx context.Context,
	profilesToUse []string,
	timeRange int,
) (string, string, string, string) {
	if len(profilesToUse) > 0 {
		sampleProfile := profilesToUse[0]
		sampleCostData, err := uc.awsRepo.GetCostData(ctx, sampleProfile, &timeRange, nil)
		if err == nil {
			previousPeriodName := sampleCostData.PreviousPeriodName
			currentPeriodName := sampleCostData.CurrentPeriodName
			previousPeriodDates := fmt.Sprintf("%s to %s",
				sampleCostData.PreviousPeriodStart.Format("2006-01-02"),
				sampleCostData.PreviousPeriodEnd.Format("2006-01-02"))
			currentPeriodDates := fmt.Sprintf("%s to %s",
				sampleCostData.CurrentPeriodStart.Format("2006-01-02"),
				sampleCostData.CurrentPeriodEnd.Format("2006-01-02"))
			return previousPeriodName, currentPeriodName, previousPeriodDates, currentPeriodDates
		}
	}
	return "Last Month Due", "Current Month Cost", "N/A", "N/A"
}

// createDisplayTable cria e configura a tabela de exibição com nomes de colunas dinâmicos.
func (uc *DashboardUseCase) createDisplayTable(
	previousPeriodDates string,
	currentPeriodDates string,
	previousPeriodName string,
	currentPeriodName string,
) types.TableInterface {
	table := uc.console.CreateTable()

	table.AddColumn("AWS Account Profile")
	table.AddColumn(fmt.Sprintf("%s\n(%s)", previousPeriodName, previousPeriodDates))
	table.AddColumn(fmt.Sprintf("%s\n(%s)", currentPeriodName, currentPeriodDates))
	table.AddColumn("Cost By Service")
	table.AddColumn("Budget Status")
	table.AddColumn("EC2 Instance Summary")

	return table
}

// generateDashboardData busca, processa e prepara os dados principais do dashboard.
func (uc *DashboardUseCase) generateDashboardData(
	ctx context.Context,
	profilesToUse []string,
	userRegions []string,
	timeRange int,
	args *types.CLIArgs,
	table types.TableInterface,
	status types.StatusHandle,
) ([]entity.ProfileData, error) {
	exportData := []entity.ProfileData{}

	if args.Combine {
		accountProfiles := make(map[string][]string)

		for _, profile := range profilesToUse {
			accountID, err := uc.awsRepo.GetAccountID(ctx, profile)
			if err != nil {
				uc.console.LogError("Error checking account ID for profile %s: %s", profile, err)
				continue
			}

			accountProfiles[accountID] = append(accountProfiles[accountID], profile)
		}

		//progress := uc.console.Progress(maps.Keys(accountProfiles))

		progressTotal := len(accountProfiles) * 5 // Multiplicamos por 5 para ter mais granularidade
		progress := uc.console.ProgressWithTotal(progressTotal)

		for accountID, profiles := range accountProfiles {
			// Atualize o status com informações sobre a conta atual
			status.Update(fmt.Sprintf("Processing account %s...", accountID))

			var profileData entity.ProfileData

			if len(profiles) > 1 {
				// Divida o processamento em etapas com atualizações incrementais
				profileData = uc.processCombinedProfilesWithProgress(ctx, accountID, profiles, userRegions, timeRange, args.Tag, progress, status)
			} else {
				// Processe um único perfil com atualizações incrementais
				profileData = uc.ProcessSingleProfileWithProgress(ctx, profiles[0], userRegions, timeRange, args.Tag, progress, status)
			}

			exportData = append(exportData, profileData)
			uc.addProfileToTable(table, profileData)
		}

		progress.Stop()
	} else {
		// Crie uma barra de progresso com mais granularidade
		progressTotal := len(profilesToUse) * 5 // Multiplicamos por 5 para ter etapas por perfil
		progress := uc.console.ProgressWithTotal(progressTotal)

		for _, profile := range profilesToUse {
			// Atualize o status com informações sobre o perfil atual
			status.Update(fmt.Sprintf("Processing profile %s...", profile))

			// Processe um único perfil com atualizações incrementais
			profileData := uc.ProcessSingleProfileWithProgress(ctx, profile, userRegions, timeRange, args.Tag, progress, status)
			exportData = append(exportData, profileData)
			uc.addProfileToTable(table, profileData)
		}

		progress.Stop()
	}

	return exportData, nil
}

// Função para processar um perfil com atualizações de progresso
func (uc *DashboardUseCase) ProcessSingleProfileWithProgress(
	ctx context.Context,
	profile string,
	userRegions []string,
	timeRange int,
	tags []string,
	progress types.ProgressHandle,
	status types.StatusHandle,
) entity.ProfileData {
	var profileData entity.ProfileData
	profileData.Profile = profile
	profileData.Success = false

	// Etapa 1: Obter dados de conta
	status.Update(fmt.Sprintf("Getting account data for %s...", profile))
	accountID, err := uc.awsRepo.GetAccountID(ctx, profile)
	if err == nil {
		profileData.AccountID = accountID
	}
	progress.Increment() // 1/5

	// Etapa 2: Obter dados de custo
	status.Update(fmt.Sprintf("Getting cost data for %s...", profile))
	costData, err := uc.awsRepo.GetCostData(ctx, profile, &timeRange, tags)
	if err != nil {
		profileData.Error = err.Error()
		// Incrementar o progresso para o restante das etapas
		progress.Increment() // 2/5
		progress.Increment() // 3/5
		progress.Increment() // 4/5
		progress.Increment() // 5/5
		return profileData
	}
	progress.Increment() // 2/5

	// Etapa 3: Definir regiões e obter resumo de EC2
	status.Update(fmt.Sprintf("Getting EC2 data for %s...", profile))
	regions := userRegions
	if len(regions) == 0 {
		regions, err = uc.awsRepo.GetAccessibleRegions(ctx, profile)
		if err != nil {
			profileData.Error = err.Error()
			progress.Increment() // 3/5
			progress.Increment() // 4/5
			progress.Increment() // 5/5
			return profileData
		}
	}
	progress.Increment() // 3/5

	// Obter resumo das instâncias EC2
	ec2Summary, err := uc.awsRepo.GetEC2Summary(ctx, profile, regions)
	if err != nil {
		profileData.Error = err.Error()
		progress.Increment() // 4/5
		progress.Increment() // 5/5
		return profileData
	}
	progress.Increment() // 4/5

	// Etapa 4: Processar e formatar os dados
	status.Update(fmt.Sprintf("Processing data for %s...", profile))

	// Processa custos por serviço
	serviceCosts, serviceCostsFormatted := uc.processServiceCosts(costData)

	// Formata informações do orçamento
	budgetInfo := uc.formatBudgetInfo(costData.Budgets)

	// Formata resumo do EC2
	ec2SummaryFormatted := uc.formatEC2Summary(ec2Summary)

	// Calcula alteração percentual no custo total
	var percentChange *float64
	if costData.LastMonthCost > 0.01 {
		change := ((costData.CurrentMonthCost - costData.LastMonthCost) / costData.LastMonthCost) * 100.0
		percentChange = &change
	} else if costData.CurrentMonthCost < 0.01 {
		change := 0.0
		percentChange = &change
	}
	progress.Increment() // 5/5

	// Preenche o dado do perfil
	profileData = entity.ProfileData{
		Profile:               profile,
		AccountID:             costData.AccountID,
		LastMonth:             costData.LastMonthCost,
		CurrentMonth:          costData.CurrentMonthCost,
		ServiceCosts:          serviceCosts,
		ServiceCostsFormatted: serviceCostsFormatted,
		BudgetInfo:            budgetInfo,
		EC2Summary:            ec2Summary,
		EC2SummaryFormatted:   ec2SummaryFormatted,
		Success:               true,
		CurrentPeriodName:     costData.CurrentPeriodName,
		PreviousPeriodName:    costData.PreviousPeriodName,
		PercentChangeInCost:   percentChange,
	}

	return profileData
}

// processCombinedProfilesWithProgress processa múltiplos perfis da mesma conta AWS,
// atualizando o progresso e status conforme avança.
func (uc *DashboardUseCase) processCombinedProfilesWithProgress(
	ctx context.Context,
	accountID string,
	profiles []string,
	userRegions []string,
	timeRange int,
	tags []string,
	progress types.ProgressHandle,
	status types.StatusHandle,
) entity.ProfileData {
	primaryProfile := profiles[0]
	profilesStr := strings.Join(profiles, ", ")

	// Inicializa os dados do perfil
	accountCostData := entity.CostData{
		AccountID:                 accountID,
		CurrentMonthCost:          0.0,
		LastMonthCost:             0.0,
		CurrentMonthCostByService: []entity.ServiceCost{},
		Budgets:                   []entity.BudgetInfo{},
		CurrentPeriodName:         "Current month",
		PreviousPeriodName:        "Last month",
		TimeRange:                 timeRange,
	}

	profileData := entity.ProfileData{
		Profile:               profilesStr,
		AccountID:             accountID,
		Success:               false,
		EC2Summary:            entity.EC2Summary{},
		ServiceCostsFormatted: []string{},
		BudgetInfo:            []string{},
		EC2SummaryFormatted:   []string{},
	}

	// Etapa 1: Verificar perfis e preparar-se para buscar dados
	status.Update(fmt.Sprintf("Initializing account %s with %d profiles...", accountID, len(profiles)))
	progress.Increment() // 1/5

	// Tenta obter dados de custo utilizando o primeiro perfil
	timeRangePtr := &timeRange
	if timeRange == 0 {
		timeRangePtr = nil
	}

	// Etapa 2: Buscar dados de custo
	status.Update(fmt.Sprintf("Getting cost data for account %s (via %s)...", accountID, primaryProfile))
	costData, err := uc.awsRepo.GetCostData(ctx, primaryProfile, timeRangePtr, tags)
	if err != nil {
		uc.console.LogError("Error getting cost data for account %s: %s", accountID, err)
		profileData.Error = fmt.Sprintf("Failed to process account: %s", err)

		// Incrementar o restante do progresso para manter a contagem
		progress.Increment() // 2/5
		progress.Increment() // 3/5
		progress.Increment() // 4/5
		progress.Increment() // 5/5

		return profileData
	}
	progress.Increment() // 2/5

	// Usa os dados de custo do primeiro perfil
	accountCostData = costData

	// Etapa 3: Define as regiões a serem usadas
	status.Update(fmt.Sprintf("Determining accessible regions for account %s...", accountID))
	regions := userRegions
	var regionErr error
	if len(regions) == 0 {
		regions, regionErr = uc.awsRepo.GetAccessibleRegions(ctx, primaryProfile)
		if regionErr != nil {
			uc.console.LogWarning("Error getting accessible regions: %s", regionErr)
			regions = []string{"us-east-1", "us-west-2", "eu-west-1", "ap-southeast-1"}
		}
	}
	progress.Increment() // 3/5

	// Etapa 4: Obter o resumo das instâncias EC2 de todos os perfis combinados
	status.Update(fmt.Sprintf("Getting EC2 data across all profiles for account %s...", accountID))
	ec2Summary := entity.EC2Summary{}

	// Inicializa os contadores de instâncias EC2
	ec2Summary["running"] = 0
	ec2Summary["stopped"] = 0

	// Combina dados de EC2 de todos os perfis
	for _, profile := range profiles {
		// Atualiza o status para mostrar o perfil atual
		status.Update(fmt.Sprintf("Getting EC2 data for profile %s in account %s...", pterm.FgCyan.Sprint(profile), pterm.FgCyan.Sprint(accountID)))

		profileEC2Summary, err := uc.awsRepo.GetEC2Summary(ctx, profile, regions)
		if err != nil {
			uc.console.LogWarning("Error getting EC2 summary for profile %s: %s", profile, err)
			continue
		}

		// Combina os resumos de EC2
		for state, count := range profileEC2Summary {
			if _, exists := ec2Summary[state]; exists {
				ec2Summary[state] += count
			} else {
				ec2Summary[state] = count
			}
		}
	}
	progress.Increment() // 4/5

	// Etapa 5: Processar e formatar todos os dados
	status.Update(fmt.Sprintf("Processing combined data for account %s...", accountID))

	// Processa custos por serviço
	serviceCosts, serviceCostsFormatted := uc.processServiceCosts(accountCostData)

	// Formata informações de orçamento
	budgetInfo := uc.formatBudgetInfo(accountCostData.Budgets)

	// Formata resumo de EC2
	ec2SummaryFormatted := uc.formatEC2Summary(ec2Summary)

	// Calcula a alteração percentual no custo total
	var percentChange *float64
	if accountCostData.LastMonthCost > 0.01 {
		change := ((accountCostData.CurrentMonthCost - accountCostData.LastMonthCost) / accountCostData.LastMonthCost) * 100.0
		percentChange = &change
	} else if accountCostData.CurrentMonthCost < 0.01 && accountCostData.LastMonthCost < 0.01 {
		change := 0.0
		percentChange = &change
	}
	progress.Increment() // 5/5

	// Preenche os dados do perfil combinado
	profileData.Success = true
	profileData.LastMonth = accountCostData.LastMonthCost
	profileData.CurrentMonth = accountCostData.CurrentMonthCost
	profileData.ServiceCosts = serviceCosts
	profileData.ServiceCostsFormatted = serviceCostsFormatted
	profileData.BudgetInfo = budgetInfo
	profileData.EC2Summary = ec2Summary
	profileData.EC2SummaryFormatted = ec2SummaryFormatted
	profileData.CurrentPeriodName = accountCostData.CurrentPeriodName
	profileData.PreviousPeriodName = accountCostData.PreviousPeriodName
	profileData.PercentChangeInCost = percentChange

	// Log de sucesso
	uc.console.LogSuccess("Successfully processed combined data for account %s with %d profiles", accountID, len(profiles))

	return profileData
}

// addProfileToTable adiciona dados do perfil à tabela de exibição.
func (uc *DashboardUseCase) addProfileToTable(table types.TableInterface, profileData entity.ProfileData) {
	if profileData.Success {
		percentageChange := profileData.PercentChangeInCost
		changeText := ""

		if percentageChange != nil {
			if *percentageChange > 0 {
				changeText = fmt.Sprintf("\n\n%s", pterm.FgRed.Sprintf("⬆ %.2f%%", *percentageChange))
			} else if *percentageChange < 0 {
				changeText = fmt.Sprintf("\n\n%s", pterm.FgGreen.Sprintf("⬇ %.2f%%", math.Abs(*percentageChange)))
			} else {
				changeText = fmt.Sprintf("\n\n%s", pterm.FgYellow.Sprintf("➡ 0.00%%"))
			}
		}

		currentMonthWithChange := fmt.Sprintf("%s%s",
			pterm.NewStyle(pterm.FgRed, pterm.Bold).Sprintf("$%.2f", profileData.CurrentMonth),
			changeText)

		// Preparando textos formatados para cada coluna
		profileText := pterm.FgMagenta.Sprintf("Profile: %s\nAccount: %s", profileData.Profile, profileData.AccountID)
		lastMonthText := pterm.NewStyle(pterm.FgRed, pterm.Bold).Sprintf("$%.2f", profileData.LastMonth)
		servicesText := pterm.FgGreen.Sprintf("%s", strings.Join(profileData.ServiceCostsFormatted, "\n"))
		budgetText := pterm.FgYellow.Sprintf("%s", strings.Join(profileData.BudgetInfo, "\n\n"))

		// Adicionando a linha à tabela
		table.AddRow(
			profileText,
			lastMonthText,
			currentMonthWithChange,
			servicesText,
			budgetText,
			strings.Join(profileData.EC2SummaryFormatted, "\n"),
		)
	} else {
		table.AddRow(
			pterm.FgMagenta.Sprintf("%s", profileData.Profile),
			pterm.FgRed.Sprint("Error"),
			pterm.FgRed.Sprint("Error"),
			pterm.FgRed.Sprintf("Failed to process profile: %s", profileData.Error),
			pterm.FgRed.Sprint("N/A"),
			pterm.FgRed.Sprint("N/A"),
		)
	}
}

func (uc *DashboardUseCase) processCombinedProfiles(
	ctx context.Context,
	accountID string,
	profiles []string,
	userRegions []string,
	timeRange int,
	tags []string,
) entity.ProfileData {
	primaryProfile := profiles[0]

	// Inicializa os dados do perfil
	accountCostData := entity.CostData{
		AccountID:                 accountID,
		CurrentMonthCost:          0.0,
		LastMonthCost:             0.0,
		CurrentMonthCostByService: []entity.ServiceCost{},
		Budgets:                   []entity.BudgetInfo{},
		CurrentPeriodName:         "Current month",
		PreviousPeriodName:        "Last month",
		TimeRange:                 timeRange,
	}

	profileData := entity.ProfileData{
		Profile:               strings.Join(profiles, ", "),
		AccountID:             accountID,
		Success:               false,
		EC2Summary:            entity.EC2Summary{},
		ServiceCostsFormatted: []string{},
		BudgetInfo:            []string{},
		EC2SummaryFormatted:   []string{},
	}

	// Tenta obter dados de custo utilizando o primeiro perfil
	timeRangePtr := &timeRange
	if timeRange == 0 {
		timeRangePtr = nil
	}

	costData, err := uc.awsRepo.GetCostData(ctx, primaryProfile, timeRangePtr, tags)
	if err != nil {
		uc.console.LogError("Error getting cost data for account %s: %s", accountID, err)
		profileData.Error = fmt.Sprintf("Failed to process account: %s", err)
		return profileData
	}

	// Usa os dados de custo do primeiro perfil
	accountCostData = costData

	// Define as regiões a serem usadas
	regions := userRegions
	var regionErr error
	if len(regions) == 0 {
		regions, regionErr = uc.awsRepo.GetAccessibleRegions(ctx, primaryProfile)
		if regionErr != nil {
			uc.console.LogWarning("Error getting accessible regions: %s", regionErr)
			regions = []string{"us-east-1", "us-west-2", "eu-west-1", "ap-southeast-1"}
		}
	}

	// Obtém o resumo das instâncias EC2 usando o primeiro perfil
	ec2Summary, err := uc.awsRepo.GetEC2Summary(ctx, primaryProfile, regions)
	if err != nil {
		uc.console.LogWarning("Error getting EC2 summary: %s", err)
		ec2Summary = entity.EC2Summary{"running": 0, "stopped": 0}
	}

	// Processa custos por serviço
	serviceCosts, serviceCostsFormatted := uc.processServiceCosts(accountCostData)

	// Formata informações de orçamento
	budgetInfo := uc.formatBudgetInfo(accountCostData.Budgets)

	// Formata resumo de EC2
	ec2SummaryFormatted := uc.formatEC2Summary(ec2Summary)

	// Calcula a alteração percentual no custo total
	var percentChange *float64
	if accountCostData.LastMonthCost > 0.01 {
		change := ((accountCostData.CurrentMonthCost - accountCostData.LastMonthCost) / accountCostData.LastMonthCost) * 100.0
		percentChange = &change
	} else if accountCostData.CurrentMonthCost < 0.01 && accountCostData.LastMonthCost < 0.01 {
		change := 0.0
		percentChange = &change
	}

	// Preenche os dados do perfil combinado
	profileData.Success = true
	profileData.LastMonth = accountCostData.LastMonthCost
	profileData.CurrentMonth = accountCostData.CurrentMonthCost
	profileData.ServiceCosts = serviceCosts
	profileData.ServiceCostsFormatted = serviceCostsFormatted
	profileData.BudgetInfo = budgetInfo
	profileData.EC2Summary = ec2Summary
	profileData.EC2SummaryFormatted = ec2SummaryFormatted
	profileData.CurrentPeriodName = accountCostData.CurrentPeriodName
	profileData.PreviousPeriodName = accountCostData.PreviousPeriodName
	profileData.PercentChangeInCost = percentChange

	return profileData
}

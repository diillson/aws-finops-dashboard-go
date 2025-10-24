package usecase

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/diillson/aws-finops-dashboard-go/internal/domain/entity"
	"github.com/diillson/aws-finops-dashboard-go/internal/domain/repository"
	"github.com/diillson/aws-finops-dashboard-go/internal/shared/types"
	"github.com/pterm/pterm"
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

// RunDashboard é o ponto de entrada principal do caso de uso.
func (uc *DashboardUseCase) RunDashboard(ctx context.Context, args *types.CLIArgs) error {
	if err := uc.mergeConfig(args); err != nil {
		return fmt.Errorf("failed to process configuration: %w", err)
	}

	profileGroups, err := uc.initializeProfiles(ctx, args)
	if err != nil {
		return err
	}
	if len(profileGroups) == 0 {
		uc.console.LogWarning("No profiles to process.")
		return nil
	}

	// Ordem de prioridade de relatórios específicos
	if args.S3Audit {
		return uc.runS3LifecycleAudit(ctx, profileGroups, args)
	}
	if args.LogsAudit {
		return uc.runCloudWatchLogsAudit(ctx, profileGroups, args)
	}
	if args.Commitments {
		return uc.runCommitmentsReport(ctx, profileGroups, args)
	}
	if args.Audit {
		return uc.runAuditReport(ctx, profileGroups, args)
	}
	if args.FullAudit {
		return uc.runFullAuditReport(ctx, profileGroups, args)
	}
	if args.Trend {
		return uc.runTrendAnalysis(ctx, profileGroups, args)
	}
	if args.Transfer {
		return uc.runDataTransferDeepDive(ctx, profileGroups, args)
	}

	return uc.runCostDashboard(ctx, profileGroups, args)
}

// runCostDashboard executa o dashboard de custos principal.
func (uc *DashboardUseCase) runCostDashboard(ctx context.Context, profileGroups []entity.ProfileGroup, args *types.CLIArgs) error {
	status := uc.console.Status("Initializing dashboard...")
	defer status.Stop() // Defer é seguro aqui, pois a impressão da tabela ocorre depois.

	var timeRange *int
	if args.TimeRange != nil && *args.TimeRange > 0 {
		timeRange = args.TimeRange
	}

	sampleProfile := profileGroups[0].Profiles[0]
	prevName, currName, prevDates, currDates := uc.getDisplayTablePeriodInfo(ctx, sampleProfile, timeRange)

	table := uc.createDisplayTable(prevDates, currDates, prevName, currName)

	status.Update("Fetching AWS data concurrently...")

	results := uc.generateDashboardData(ctx, profileGroups, args)

	// Parar o spinner explicitamente aqui garante que ele desapareça
	// antes de qualquer outra impressão.
	status.Stop()

	// O MultiPrinter dentro de generateDashboardData já parou e limpou suas linhas.

	sort.Slice(results, func(i, j int) bool {
		return results[i].Profile < results[j].Profile
	})

	for _, data := range results {
		uc.addProfileToTable(table, data)
	}

	uc.console.Print("\n" + table.Render())

	if args.ReportName != "" {
		uc.exportCostDashboardReports(results, args, prevDates, currDates)
	}

	return nil
}

func (uc *DashboardUseCase) runCloudWatchLogsAudit(ctx context.Context, profileGroups []entity.ProfileGroup, args *types.CLIArgs) error {
	uc.console.LogInfo("Auditing CloudWatch Logs retention...")

	livePrinter, _ := uc.console.GetMultiPrinter().Start()
	defer livePrinter.Stop()

	type row struct {
		Profile   string
		AccountID string
		Audit     entity.CloudWatchLogsAudit
		Err       error
	}

	results := make([]row, 0, len(profileGroups))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, group := range profileGroups {
		wg.Add(1)
		go func(g entity.ProfileGroup) {
			defer wg.Done()

			// Uma barra por perfil: 2 passos (List regions + Fetch logs)
			bar := uc.console.NewProgressbar(2, fmt.Sprintf("Logs Audit: %s", g.Identifier))
			bar.Start()

			profile := g.Profiles[0]
			regions := args.Regions
			if len(regions) == 0 {
				rs, _ := uc.awsRepo.GetAccessibleRegions(ctx, profile)
				regions = rs
			}
			bar.Increment()

			logGroups, err := uc.awsRepo.GetCloudWatchLogGroups(ctx, profile, regions)
			if err != nil {
				mu.Lock()
				results = append(results, row{Profile: g.Identifier, Err: err})
				mu.Unlock()
				bar.Increment()
				return
			}
			bar.Increment()

			// Filtra grupos sem retenção e ordena por tamanho desc
			noRetention := make([]entity.CloudWatchLogGroupInfo, 0, len(logGroups))
			var totalBytes int64
			for _, lg := range logGroups {
				totalBytes += lg.StoredBytes
				if lg.RetentionDays == 0 {
					noRetention = append(noRetention, lg)
				}
			}
			sort.Slice(noRetention, func(i, j int) bool {
				return noRetention[i].StoredBytes > noRetention[j].StoredBytes
			})

			// Top N mais pesados
			const topN = 10
			top := noRetention
			if len(noRetention) > topN {
				top = noRetention[:topN]
			}

			accountID, _ := uc.awsRepo.GetAccountID(ctx, profile)
			audit := entity.CloudWatchLogsAudit{
				Profile:            g.Identifier,
				AccountID:          accountID,
				NoRetentionCount:   len(noRetention),
				NoRetentionTopN:    top,
				TotalStoredGB:      float64(totalBytes) / (1024.0 * 1024.0 * 1024.0),
				RecommendedMessage: "Set retention days per environment (e.g., 7/14/30) to avoid unlimited storage growth.",
			}

			mu.Lock()
			results = append(results, row{
				Profile:   g.Identifier,
				AccountID: accountID,
				Audit:     audit,
			})
			mu.Unlock()
		}(group)
	}
	wg.Wait()

	// Ordena por perfil
	sort.Slice(results, func(i, j int) bool { return results[i].Profile < results[j].Profile })

	// Monta a tabela
	table := uc.console.CreateTable()
	table.AddColumn("Profile")
	table.AddColumn("Account ID")
	table.AddColumn("No Retention (count)")
	table.AddColumn("Total Stored (GB)")
	table.AddColumn("Top No-Retention Groups (Region | Name | GB)")

	for _, r := range results {
		if r.Err != nil {
			table.AddRow(
				pterm.FgMagenta.Sprint(r.Profile),
				"N/A",
				"N/A",
				pterm.FgRed.Sprintf("Error: %v", r.Err),
				"-",
			)
			continue
		}
		var lines []string
		limit := len(r.Audit.NoRetentionTopN)
		if limit > 5 {
			limit = 5
		}
		for i := 0; i < limit; i++ {
			lg := r.Audit.NoRetentionTopN[i]
			gb := float64(lg.StoredBytes) / (1024.0 * 1024.0 * 1024.0)
			lines = append(lines, fmt.Sprintf("%s | %s | %.2f GB", lg.Region, lg.GroupName, gb))
		}
		if len(r.Audit.NoRetentionTopN) > limit {
			lines = append(lines, fmt.Sprintf("... (+%d more)", len(r.Audit.NoRetentionTopN)-limit))
		}

		table.AddRow(
			pterm.FgMagenta.Sprint(r.Profile),
			r.AccountID,
			fmt.Sprintf("%d", r.Audit.NoRetentionCount),
			fmt.Sprintf("%.2f", r.Audit.TotalStoredGB),
			strings.Join(lines, "\n"),
		)
	}
	uc.console.Println("\n" + table.Render())

	// Export
	if args.ReportName != "" {
		uc.console.LogInfo("Exporting CloudWatch Logs audit reports...")
		audits := make([]entity.CloudWatchLogsAudit, 0, len(results))
		for _, r := range results {
			if r.Err == nil {
				audits = append(audits, r.Audit)
			}
		}
		for _, reportType := range args.ReportType {
			switch strings.ToLower(reportType) {
			case "csv":
				path, err := uc.exportRepo.ExportLogsAuditToCSV(audits, args.ReportName, args.Dir)
				if err != nil {
					uc.console.LogError("Failed to export logs audit CSV: %v", err)
				} else {
					uc.console.LogSuccess("Logs audit CSV saved to: %s", path)
				}
			case "json":
				path, err := uc.exportRepo.ExportLogsAuditToJSON(audits, args.ReportName, args.Dir)
				if err != nil {
					uc.console.LogError("Failed to export logs audit JSON: %v", err)
				} else {
					uc.console.LogSuccess("Logs audit JSON saved to: %s", path)
				}
			case "pdf":
				path, err := uc.exportRepo.ExportLogsAuditToPDF(audits, args.ReportName, args.Dir)
				if err != nil {
					uc.console.LogError("Failed to export logs audit PDF: %v", err)
				} else {
					uc.console.LogSuccess("Logs audit PDF saved to: %s", path)
				}
			}
		}
	}

	return nil
}

type profileJob struct {
	Group       entity.ProfileGroup
	Args        *types.CLIArgs
	ProgressBar *pterm.ProgressbarPrinter
}

func (uc *DashboardUseCase) runDataTransferDeepDive(ctx context.Context, profileGroups []entity.ProfileGroup, args *types.CLIArgs) error {
	uc.console.LogInfo("Analysing data transfer costs...")

	// MultiPrinter para progress bars por perfil
	livePrinter, _ := uc.console.GetMultiPrinter().Start()
	defer livePrinter.Stop()

	type row struct {
		Profile   string
		AccountID string
		Report    entity.DataTransferReport
		Err       error
	}
	results := make([]row, 0, len(profileGroups))

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, group := range profileGroups {
		wg.Add(1)
		go func(g entity.ProfileGroup) {
			defer wg.Done()

			bar := uc.console.NewProgressbar(1, fmt.Sprintf("Data Transfer: %s", g.Identifier))
			bar.Start()

			// Usa o primeiro perfil real do grupo (tal qual trend)
			profile := g.Profiles[0]

			var timeRange *int
			if args.TimeRange != nil && *args.TimeRange > 0 {
				timeRange = args.TimeRange
			}

			report, err := uc.awsRepo.GetDataTransferBreakdown(ctx, profile, timeRange, args.Tag)
			if err != nil {
				mu.Lock()
				results = append(results, row{Profile: g.Identifier, AccountID: "", Err: err})
				mu.Unlock()
				bar.Increment()
				return
			}

			accountID := report.AccountID
			mu.Lock()
			results = append(results, row{Profile: g.Identifier, AccountID: accountID, Report: report})
			mu.Unlock()
			bar.Increment()
		}(group)
	}
	wg.Wait()

	// Ordena por perfil
	sort.Slice(results, func(i, j int) bool { return results[i].Profile < results[j].Profile })

	// Monta tabela agregada por categoria
	table := uc.console.CreateTable()
	table.AddColumn("Profile")
	table.AddColumn("Account ID")
	table.AddColumn("Period")
	table.AddColumn("Total")
	table.AddColumn("Internet")
	table.AddColumn("Inter-Region")
	table.AddColumn("Cross-AZ/Regional")
	table.AddColumn("NAT Gateway")
	table.AddColumn("Other")

	getCat := func(cats []entity.DataTransferCategoryCost, name string) float64 {
		for _, c := range cats {
			if c.Category == name {
				return c.Cost
			}
		}
		return 0
	}

	for _, r := range results {
		if r.Err != nil {
			table.AddRow(
				pterm.FgMagenta.Sprint(r.Profile),
				"N/A",
				"N/A",
				pterm.FgRed.Sprintf("Error: %v", r.Err),
				"-", "-", "-", "-", "-",
			)
			continue
		}

		rep := r.Report
		period := fmt.Sprintf("%s to %s", rep.PeriodStart.Format("2006-01-02"), rep.PeriodEnd.Format("2006-01-02"))
		table.AddRow(
			pterm.FgMagenta.Sprint(r.Profile),
			r.AccountID,
			period,
			fmt.Sprintf("$%.2f", rep.Total),
			fmt.Sprintf("$%.2f", getCat(rep.Categories, "Internet")),
			fmt.Sprintf("$%.2f", getCat(rep.Categories, "Inter-Region")),
			fmt.Sprintf("$%.2f", getCat(rep.Categories, "Cross-AZ/Regional")),
			fmt.Sprintf("$%.2f", getCat(rep.Categories, "NAT Gateway")),
			fmt.Sprintf("$%.2f", getCat(rep.Categories, "Other")),
		)
	}

	uc.console.Println("\n" + table.Render())

	// Exibe Top Lines (5) por perfil após a tabela
	const topShow = 5
	for _, r := range results {
		if r.Err != nil {
			continue
		}
		rep := r.Report
		uc.console.Println(pterm.FgYellow.Sprintf("\nTop data transfer lines for %s (Account: %s)", r.Profile, r.AccountID))
		lines := rep.TopLines
		limit := len(lines)
		if limit > topShow {
			limit = topShow
		}
		for i := 0; i < limit; i++ {
			l := lines[i]
			uc.console.Println(fmt.Sprintf("  - %s | %s: $%.2f", l.Service, l.UsageType, l.Cost))
		}
		if len(lines) > limit {
			uc.console.Println(fmt.Sprintf("  ... (+%d more)", len(lines)-limit))
		}
	}

	// Export dos relatórios de transferência de dados
	if args.ReportName != "" {
		uc.console.LogInfo("Exporting data transfer reports...")
		// Monta []entity.DataTransferReport
		reports := make([]entity.DataTransferReport, 0, len(results))
		for _, r := range results {
			if r.Err == nil {
				reports = append(reports, r.Report)
			}
		}
		for _, reportType := range args.ReportType {
			switch strings.ToLower(reportType) {
			case "csv":
				path, err := uc.exportRepo.ExportTransferReportToCSV(reports, args.ReportName, args.Dir)
				if err != nil {
					uc.console.LogError("Failed to export transfer report to CSV: %v", err)
				} else {
					uc.console.LogSuccess("Transfer CSV report saved to: %s", path)
				}
			case "json":
				path, err := uc.exportRepo.ExportTransferReportToJSON(reports, args.ReportName, args.Dir)
				if err != nil {
					uc.console.LogError("Failed to export transfer report to JSON: %v", err)
				} else {
					uc.console.LogSuccess("Transfer JSON report saved to: %s", path)
				}
			case "pdf":
				path, err := uc.exportRepo.ExportTransferReportToPDF(reports, args.ReportName, args.Dir)
				if err != nil {
					uc.console.LogError("Failed to export transfer report to PDF: %v", err)
				} else {
					uc.console.LogSuccess("Transfer PDF report saved to: %s", path)
				}
			}
		}
	}

	return nil
}

func (uc *DashboardUseCase) generateDashboardData(ctx context.Context, profileGroups []entity.ProfileGroup, args *types.CLIArgs) []entity.ProfileData {
	numJobs := len(profileGroups)
	jobs := make(chan profileJob, numJobs)
	results := make(chan entity.ProfileData, numJobs)

	livePrinter, _ := uc.console.GetMultiPrinter().Start()
	defer livePrinter.Stop()

	var wg sync.WaitGroup
	numWorkers := 10
	if numJobs < numJobs {
		numWorkers = numJobs
	}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				results <- uc.processProfileJob(ctx, job)
			}
		}()
	}

	for _, group := range profileGroups {
		// 3. Cria a barra de progresso (sem iniciar).
		bar := uc.console.NewProgressbar(5, group.Identifier)
		// 4. Inicia a barra de progresso. A pterm irá automaticamente associá-la ao MultiPrinter ativo.
		bar.Start()
		jobs <- profileJob{Group: group, Args: args, ProgressBar: bar}
	}
	close(jobs)

	processedData := make([]entity.ProfileData, 0, numJobs)
	for i := 0; i < numJobs; i++ {
		processedData = append(processedData, <-results)
	}

	wg.Wait()
	return processedData
}

func (uc *DashboardUseCase) processProfileJob(ctx context.Context, job profileJob) entity.ProfileData {
	var timeRange *int
	if job.Args.TimeRange != nil && *job.Args.TimeRange > 0 {
		timeRange = job.Args.TimeRange
	}

	if job.Group.IsCombined {
		return uc.processCombinedProfile(ctx, job.Group, job.Args.Regions, timeRange, job.Args.Tag, job.Args.BreakdownCosts, job.ProgressBar)
	}
	return uc.processSingleProfile(ctx, job.Group.Profiles[0], job.Args.Regions, timeRange, job.Args.Tag, job.Args.BreakdownCosts, job.ProgressBar)
}

func (uc *DashboardUseCase) processSingleProfile(ctx context.Context, profile string, userRegions []string, timeRange *int, tags []string, breakdown bool, progress *pterm.ProgressbarPrinter) entity.ProfileData {
	data := entity.ProfileData{Profile: profile, Success: false}
	progress.Increment()

	// Passa a flag 'breakdown' para o repositório
	costData, err := uc.awsRepo.GetCostData(ctx, profile, timeRange, tags, breakdown)
	if err != nil {
		data.Err = fmt.Errorf("failed to get cost data: %w", err)
		return data
	}
	progress.Increment()

	regions := userRegions
	if len(regions) == 0 {
		regions, _ = uc.awsRepo.GetAccessibleRegions(ctx, profile)
	}
	progress.Increment()

	ec2Summary, err := uc.awsRepo.GetEC2Summary(ctx, profile, regions)
	if err != nil {
		data.Err = fmt.Errorf("failed to get EC2 summary: %w", err)
		return data
	}
	progress.Increment()

	uc.populateProfileData(&data, &costData, ec2Summary)
	progress.Increment()

	return data
}

func (uc *DashboardUseCase) processCombinedProfile(ctx context.Context, group entity.ProfileGroup, userRegions []string, timeRange *int, tags []string, breakdown bool, progress *pterm.ProgressbarPrinter) entity.ProfileData {
	data := entity.ProfileData{Profile: group.Identifier, AccountID: group.AccountID, Success: false}
	primaryProfile := group.Profiles[0]
	progress.Increment()

	// Passa a flag 'breakdown' para o repositório
	costData, err := uc.awsRepo.GetCostData(ctx, primaryProfile, timeRange, tags, breakdown)
	if err != nil {
		data.Err = fmt.Errorf("failed to get cost data for account: %w", err)
		return data
	}
	progress.Increment()

	regions := userRegions
	if len(regions) == 0 {
		regions, _ = uc.awsRepo.GetAccessibleRegions(ctx, primaryProfile)
	}
	progress.Increment()

	combinedEC2Summary := make(entity.EC2Summary)
	var ec2Wg sync.WaitGroup
	var ec2Mu sync.Mutex

	for _, p := range group.Profiles {
		ec2Wg.Add(1)
		go func(prof string) {
			defer ec2Wg.Done()
			summary, err := uc.awsRepo.GetEC2Summary(ctx, prof, regions)
			if err != nil {
				return
			}
			ec2Mu.Lock()
			for state, count := range summary {
				combinedEC2Summary[state] += count
			}
			ec2Mu.Unlock()
		}(p)
	}
	ec2Wg.Wait()
	progress.Increment()

	uc.populateProfileData(&data, &costData, combinedEC2Summary)
	progress.Increment()

	return data
}

func (uc *DashboardUseCase) populateProfileData(data *entity.ProfileData, costData *entity.CostData, ec2Summary entity.EC2Summary) {
	data.AccountID = costData.AccountID
	data.LastMonth = costData.LastMonthCost
	data.CurrentMonth = costData.CurrentMonthCost
	data.CurrentPeriodName = costData.CurrentPeriodName
	data.PreviousPeriodName = costData.PreviousPeriodName
	data.ServiceCosts = costData.CurrentMonthCostByService
	data.ServiceCostsFormatted = uc.formatServiceCosts(costData.CurrentMonthCostByService)
	data.BudgetInfo = uc.formatBudgetInfo(costData.Budgets)
	data.EC2Summary = ec2Summary
	data.EC2SummaryFormatted = uc.formatEC2Summary(ec2Summary)
	data.PercentChangeInCost = uc.calculatePercentageChange(data.CurrentMonth, data.LastMonth)
	data.Success = true
}

func (uc *DashboardUseCase) mergeConfig(args *types.CLIArgs) error {
	if args.ConfigFile == "" {
		return nil
	}

	cfg, err := uc.configRepo.LoadConfigFile(args.ConfigFile)
	if err != nil {
		return err
	}

	if len(args.Profiles) == 0 {
		args.Profiles = cfg.Profiles
	}
	if len(args.Regions) == 0 {
		args.Regions = cfg.Regions
	}
	if !args.All {
		args.All = cfg.All
	}
	if !args.Combine {
		args.Combine = cfg.Combine
	}
	if args.ReportName == "" {
		args.ReportName = cfg.ReportName
	}
	if len(args.ReportType) <= 1 && (len(args.ReportType) == 0 || args.ReportType[0] == "csv") {
		args.ReportType = cfg.ReportType
	}
	if args.Dir == "" {
		args.Dir = cfg.Dir
	}
	if args.TimeRange == nil && cfg.TimeRange > 0 {
		val := cfg.TimeRange
		args.TimeRange = &val
	}
	if len(args.Tag) == 0 {
		args.Tag = cfg.Tag
	}
	if !args.Trend {
		args.Trend = cfg.Trend
	}
	if !args.Audit {
		args.Audit = cfg.Audit
	}

	return nil
}

func (uc *DashboardUseCase) initializeProfiles(ctx context.Context, args *types.CLIArgs) ([]entity.ProfileGroup, error) {
	availableProfiles := uc.awsRepo.GetAWSProfiles()
	var profilesToScan []string

	if args.All {
		profilesToScan = availableProfiles
	} else if len(args.Profiles) > 0 {
		// Validar perfis
		for _, p := range args.Profiles {
			found := false
			for _, ap := range availableProfiles {
				if p == ap {
					profilesToScan = append(profilesToScan, p)
					found = true
					break
				}
			}
			if !found {
				uc.console.LogWarning("Profile '%s' not found.", p)
			}
		}
	} else {
		// Padrão
		for _, ap := range availableProfiles {
			if ap == "default" {
				profilesToScan = []string{"default"}
				break
			}
		}
		if len(profilesToScan) == 0 && len(availableProfiles) > 0 {
			profilesToScan = availableProfiles
			uc.console.LogInfo("No default profile found. Using all %d available profiles.", len(availableProfiles))
		}
	}

	if len(profilesToScan) == 0 {
		return nil, types.ErrNoValidProfilesFound
	}

	// --- INÍCIO DA MELHORIA: VERIFICAÇÃO DE CREDENCIAIS ---
	// Verifica a validade das credenciais usando o primeiro perfil da lista.
	// Isso fornece um feedback rápido ao usuário se as credenciais expiraram.
	uc.console.LogInfo("Verifying AWS credentials using profile '%s'...", profilesToScan[0])
	_, err := uc.awsRepo.GetAccountID(ctx, profilesToScan[0])
	if err != nil {
		// Retorna um erro muito mais claro para o usuário!
		return nil, fmt.Errorf(
			"credential validation failed for profile '%s'. Reason: %w. Please check your AWS credentials or session token",
			profilesToScan[0],
			err,
		)
	}
	uc.console.LogSuccess("AWS credentials verified.")
	// --- FIM DA MELHORIA ---

	if !args.Combine {
		groups := make([]entity.ProfileGroup, len(profilesToScan))
		for i, p := range profilesToScan {
			groups[i] = entity.ProfileGroup{Identifier: p, Profiles: []string{p}, IsCombined: false}
		}
		return groups, nil
	}

	// Lógica para combinar perfis (continua a mesma)
	accountToProfiles := make(map[string][]string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range profilesToScan {
		wg.Add(1)
		go func(prof string) {
			defer wg.Done()
			// Reutiliza a chamada GetAccountID que já sabemos que funciona para o primeiro perfil
			accID, err := uc.awsRepo.GetAccountID(ctx, prof)
			if err != nil {
				uc.console.LogWarning("Could not get Account ID for profile '%s', skipping. Error: %v", prof, err)
				return
			}
			mu.Lock()
			accountToProfiles[accID] = append(accountToProfiles[accID], prof)
			mu.Unlock()
		}(p)
	}
	wg.Wait()

	groups := make([]entity.ProfileGroup, 0, len(accountToProfiles))
	for accID, profiles := range accountToProfiles {
		sort.Strings(profiles)
		groups = append(groups, entity.ProfileGroup{
			Identifier: strings.Join(profiles, ", "),
			AccountID:  accID,
			Profiles:   profiles,
			IsCombined: true,
		})
	}
	return groups, nil
}

func (uc *DashboardUseCase) getDisplayTablePeriodInfo(ctx context.Context, profile string, timeRange *int) (string, string, string, string) {
	costData, err := uc.awsRepo.GetCostData(ctx, profile, timeRange, nil, false)

	if err == nil {
		pFormat := "2006-01-02"
		prevDates := fmt.Sprintf("%s to %s", costData.PreviousPeriodStart.Format(pFormat), costData.PreviousPeriodEnd.Format(pFormat))
		currDates := fmt.Sprintf("%s to %s", costData.CurrentPeriodStart.Format(pFormat), costData.CurrentPeriodEnd.Format(pFormat))
		return costData.PreviousPeriodName, costData.CurrentPeriodName, prevDates, currDates
	}
	return "Last Month Due", "Current Month Cost", "N/A", "N/A"
}

func (uc *DashboardUseCase) createDisplayTable(previousPeriodDates, currentPeriodDates, previousPeriodName, currentPeriodName string) types.TableInterface {
	table := uc.console.CreateTable()
	table.AddColumn("AWS Account Profile")
	table.AddColumn(fmt.Sprintf("%s\n(%s)", previousPeriodName, previousPeriodDates))
	table.AddColumn(fmt.Sprintf("%s\n(%s)", currentPeriodName, currentPeriodDates))
	table.AddColumn("Cost By Service")
	table.AddColumn("Budget Status")
	table.AddColumn("EC2 Instance Summary")
	return table
}

func (uc *DashboardUseCase) addProfileToTable(table types.TableInterface, data entity.ProfileData) {
	if !data.Success {
		table.AddRow(
			pterm.FgMagenta.Sprint(data.Profile),
			pterm.FgRed.Sprint("Error"),
			pterm.FgRed.Sprint("Error"),
			pterm.FgRed.Sprintf("Failed: %v", data.Err),
			pterm.FgRed.Sprint("N/A"),
			pterm.FgRed.Sprint("N/A"),
		)
		return
	}

	changeText := ""
	if data.PercentChangeInCost != nil {
		val := *data.PercentChangeInCost
		if val > 0.01 {
			changeText = pterm.FgRed.Sprintf("\n\n⬆ %.2f%%", val)
		} else if val < -0.01 {
			changeText = pterm.FgGreen.Sprintf("\n\n⬇ %.2f%%", math.Abs(val))
		} else {
			changeText = pterm.FgYellow.Sprintf("\n\n➡ 0.00%%")
		}
	}

	table.AddRow(
		pterm.FgMagenta.Sprintf("Profile: %s\nAccount: %s", data.Profile, data.AccountID),
		pterm.Bold.Sprintf("$%.2f", data.LastMonth),
		fmt.Sprintf("%s%s", pterm.Bold.Sprintf("$%.2f", data.CurrentMonth), changeText),
		strings.Join(data.ServiceCostsFormatted, "\n"),
		strings.Join(data.BudgetInfo, "\n\n"),
		strings.Join(data.EC2SummaryFormatted, "\n"),
	)
}

func (uc *DashboardUseCase) exportCostDashboardReports(results []entity.ProfileData, args *types.CLIArgs, prevDates, currDates string) {
	uc.console.LogInfo("Exporting reports...")
	for _, reportType := range args.ReportType {
		switch strings.ToLower(reportType) {
		case "csv":
			path, err := uc.exportRepo.ExportToCSV(results, args.ReportName, args.Dir, prevDates, currDates)
			if err != nil {
				uc.console.LogError("Failed to export to CSV: %v", err)
			} else {
				uc.console.LogSuccess("CSV report saved to: %s", path)
			}
		case "json":
			path, err := uc.exportRepo.ExportToJSON(results, args.ReportName, args.Dir)
			if err != nil {
				uc.console.LogError("Failed to export to JSON: %v", err)
			} else {
				uc.console.LogSuccess("JSON report saved to: %s", path)
			}
		case "pdf":
			path, err := uc.exportRepo.ExportToPDF(results, args.ReportName, args.Dir, prevDates, currDates)
			if err != nil {
				uc.console.LogError("Failed to export to PDF: %v", err)
			} else {
				uc.console.LogSuccess("PDF report saved to: %s", path)
			}
		}
	}
}

// runAuditReport com progress bars por perfil (MultiPrinter) e sem updates concorrentes no spinner.
func (uc *DashboardUseCase) runAuditReport(ctx context.Context, profileGroups []entity.ProfileGroup, args *types.CLIArgs) error {
	uc.console.LogInfo("Preparing your audit report...")

	livePrinter, _ := uc.console.GetMultiPrinter().Start()
	defer livePrinter.Stop()

	// Tabela do terminal
	table := uc.console.CreateTable()
	table.AddColumn("Profile")
	table.AddColumn("Account ID")
	table.AddColumn("Budget Alerts")
	table.AddColumn("High-Cost NAT GWs")
	table.AddColumn("Unused VPC Endpoints")
	table.AddColumn("Idle Load Balancers")
	table.AddColumn("Stopped EC2")
	table.AddColumn("Unused Volumes")
	table.AddColumn("Unused Elastic IPs")
	table.AddColumn("Untagged Resources")

	var auditDataList []entity.AuditData
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, group := range profileGroups {
		wg.Add(1)
		go func(g entity.ProfileGroup) {
			defer wg.Done()

			// Uma barra por perfil (8 etapas)
			const totalSteps = 8
			bar := uc.console.NewProgressbar(totalSteps, fmt.Sprintf("Auditing: %s", g.Identifier))
			bar.Start()

			profile := g.Profiles[0]

			var timeRange *int
			if args.TimeRange != nil && *args.TimeRange > 0 {
				timeRange = args.TimeRange
			}

			regions := args.Regions
			if len(regions) == 0 {
				regions, _ = uc.awsRepo.GetAccessibleRegions(ctx, profile)
			}

			var (
				natCosts        []entity.NatGatewayCost
				idleLBs         entity.IdleLoadBalancers
				stopped         entity.StoppedEC2Instances
				unusedVols      entity.UnusedVolumes
				unusedEIPs      entity.UnusedEIPs
				untagged        entity.UntaggedResources
				unusedEndpoints entity.UnusedVpcEndpoints
				budgets         []entity.BudgetInfo
			)

			var auditWg sync.WaitGroup
			auditWg.Add(totalSteps)

			go func() {
				defer auditWg.Done()
				natCosts, _ = uc.awsRepo.GetNatGatewayCost(ctx, profile, timeRange, args.Tag)
				bar.Increment()
			}()
			go func() {
				defer auditWg.Done()
				idleLBs, _ = uc.awsRepo.GetIdleLoadBalancers(ctx, profile, regions)
				bar.Increment()
			}()
			go func() {
				defer auditWg.Done()
				stopped, _ = uc.awsRepo.GetStoppedInstances(ctx, profile, regions)
				bar.Increment()
			}()
			go func() {
				defer auditWg.Done()
				unusedVols, _ = uc.awsRepo.GetUnusedVolumes(ctx, profile, regions)
				bar.Increment()
			}()
			go func() {
				defer auditWg.Done()
				unusedEIPs, _ = uc.awsRepo.GetUnusedEIPs(ctx, profile, regions)
				bar.Increment()
			}()
			go func() {
				defer auditWg.Done()
				untagged, _ = uc.awsRepo.GetUntaggedResources(ctx, profile, regions)
				bar.Increment()
			}()
			go func() {
				defer auditWg.Done()
				unusedEndpoints, _ = uc.awsRepo.GetUnusedVpcEndpoints(ctx, profile, regions)
				bar.Increment()
			}()
			go func() { defer auditWg.Done(); budgets, _ = uc.awsRepo.GetBudgets(ctx, profile); bar.Increment() }()

			auditWg.Wait() // barra chega ao total e some (RemoveWhenDone = true)

			accountID, _ := uc.awsRepo.GetAccountID(ctx, profile)
			natCostsStr := formatNatGatewayCosts(natCosts)
			idleLBsStr := formatAuditMap(idleLBs, "Idle Load Balancers")
			stoppedStr := formatAuditMap(stopped, "Stopped Instances")
			volsStr := formatAuditMap(unusedVols, "Unused Volumes")
			eipsStr := formatAuditMap(unusedEIPs, "Unused Elastic IPs")
			untaggedStr := formatAuditMapForUntagged(untagged)
			unusedEndpointsStr := formatAuditMap(unusedEndpoints, "Unused VPC Endpoints")
			alertsStr := formatBudgetAlerts(budgets)

			mu.Lock()
			auditDataList = append(auditDataList, entity.AuditData{
				Profile:            profile,
				AccountID:          accountID,
				NatGatewayCosts:    natCostsStr,
				IdleLoadBalancers:  idleLBsStr,
				StoppedInstances:   stoppedStr,
				UnusedVolumes:      volsStr,
				UnusedEIPs:         eipsStr,
				UntaggedResources:  untaggedStr,
				UnusedVpcEndpoints: unusedEndpointsStr,
				BudgetAlerts:       alertsStr,
			})
			mu.Unlock()
		}(group)
	}
	wg.Wait()

	// Ordenação por perfil
	sort.Slice(auditDataList, func(i, j int) bool { return auditDataList[i].Profile < auditDataList[j].Profile })

	// Escreve a tabela
	for _, data := range auditDataList {
		table.AddRow(
			pterm.FgMagenta.Sprint(data.Profile),
			data.AccountID,
			data.BudgetAlerts,
			data.NatGatewayCosts,
			data.UnusedVpcEndpoints,
			data.IdleLoadBalancers,
			data.StoppedInstances,
			data.UnusedVolumes,
			data.UnusedEIPs,
			data.UntaggedResources,
		)
	}
	uc.console.Println("\n" + table.Render())

	if args.ReportName != "" {
		uc.console.LogInfo("Exporting audit reports...")
		for _, reportType := range args.ReportType {
			switch strings.ToLower(reportType) {
			case "csv":
				path, err := uc.exportRepo.ExportAuditReportToCSV(auditDataList, args.ReportName, args.Dir)
				if err != nil {
					uc.console.LogError("Failed to export audit report to CSV: %v", err)
				} else {
					uc.console.LogSuccess("Audit CSV report saved to: %s", path)
				}
			case "json":
				path, err := uc.exportRepo.ExportAuditReportToJSON(auditDataList, args.ReportName, args.Dir)
				if err != nil {
					uc.console.LogError("Failed to export audit report to JSON: %v", err)
				} else {
					uc.console.LogSuccess("Audit JSON report saved to: %s", path)
				}
			case "pdf":
				path, err := uc.exportRepo.ExportAuditReportToPDF(auditDataList, args.ReportName, args.Dir)
				if err != nil {
					uc.console.LogError("Failed to export audit report to PDF: %v", err)
				} else {
					uc.console.LogSuccess("Audit PDF report saved to: %s", path)
				}
			}
		}
	}

	return nil
}

func formatNatGatewayCosts(costs []entity.NatGatewayCost) string {
	if len(costs) == 0 {
		return "None"
	}
	var builder strings.Builder
	// Limita a exibição aos 5 mais caros para não poluir a UI
	limit := 5
	if len(costs) < limit {
		limit = len(costs)
	}

	for i := 0; i < limit; i++ {
		c := costs[i]
		builder.WriteString(pterm.FgRed.Sprintf("$%.2f", c.Cost))
		builder.WriteString(fmt.Sprintf(" - %s (%s)\n", c.ResourceID, c.Region))
	}
	return strings.TrimSpace(builder.String())
}

// Limite “de produção” para não poluir terminal/exports quando há muitos itens.
const maxItemsPerRegion = 50

// formatAuditMapForUntagged com limite por região e ordenação determinística.
func formatAuditMapForUntagged(data entity.UntaggedResources) string {
	var builder strings.Builder
	hasContent := false

	services := make([]string, 0, len(data))
	for s := range data {
		services = append(services, s)
	}
	sort.Strings(services)

	for _, service := range services {
		regions := data[service]
		if len(regions) == 0 {
			continue
		}
		hasContent = true
		builder.WriteString(pterm.FgYellow.Sprintf("%s:\n", service))

		regionNames := make([]string, 0, len(regions))
		for r := range regions {
			regionNames = append(regionNames, r)
		}
		sort.Strings(regionNames)

		for _, region := range regionNames {
			items := regions[region]
			if len(items) == 0 {
				continue
			}
			sort.Strings(items)

			builder.WriteString(pterm.FgCyan.Sprintf("  %s:\n", region))

			// Aplica limite por região
			limit := len(items)
			if limit > maxItemsPerRegion {
				limit = maxItemsPerRegion
			}
			for _, item := range items[:limit] {
				builder.WriteString(fmt.Sprintf("    - %s\n", item))
			}
			if len(items) > limit {
				builder.WriteString(fmt.Sprintf("    ... (+%d more)\n", len(items)-limit))
			}
		}
	}

	if !hasContent {
		return "None"
	}
	return builder.String()
}

// formatAuditMap genérico (Idle LBs, Stopped, Volumes, EIPs, VPC Endpoints) com limite por região.
func formatAuditMap[V ~map[string][]string](data V, title string) string {
	if len(data) == 0 {
		return "None"
	}
	var builder strings.Builder

	regions := make([]string, 0, len(data))
	for r := range data {
		regions = append(regions, r)
	}
	sort.Strings(regions)

	for _, region := range regions {
		items := data[region]
		if len(items) == 0 {
			continue
		}
		sort.Strings(items)

		builder.WriteString(pterm.FgCyan.Sprintf("%s:\n", region))

		limit := len(items)
		if limit > maxItemsPerRegion {
			limit = maxItemsPerRegion
		}
		for _, item := range items[:limit] {
			builder.WriteString(fmt.Sprintf("  - %s\n", item))
		}
		if len(items) > limit {
			builder.WriteString(fmt.Sprintf("  ... (+%d more)\n", len(items)-limit))
		}
	}
	return builder.String()
}

func formatBudgetAlerts(budgets []entity.BudgetInfo) string {
	var alerts []string
	for _, b := range budgets {
		if b.Actual > b.Limit {
			alerts = append(alerts, pterm.FgRed.Sprintf("%s: $%.2f > $%.2f", b.Name, b.Actual, b.Limit))
		}
	}
	if len(alerts) == 0 {
		return "No budgets exceeded"
	}
	return strings.Join(alerts, "\n")
}

func (uc *DashboardUseCase) runTrendAnalysis(ctx context.Context, profileGroups []entity.ProfileGroup, args *types.CLIArgs) error {
	uc.console.LogInfo("Analysing cost trends...")
	status := uc.console.Status("Fetching trend data...")
	defer status.Stop()

	for _, group := range profileGroups {
		status.Update(fmt.Sprintf("Fetching trend for %s...", group.Identifier))
		profileForAPI := group.Profiles[0] // Usa o primeiro perfil para a chamada de API

		trendData, err := uc.awsRepo.GetTrendData(ctx, profileForAPI, args.Tag)
		if err != nil {
			uc.console.LogError("Error getting trend for %s: %v", group.Identifier, err)
			continue
		}

		monthlyCosts, ok := trendData["monthly_costs"].([]entity.MonthlyCost)
		if !ok || len(monthlyCosts) == 0 {
			uc.console.LogWarning("No trend data available for %s", group.Identifier)
			continue
		}

		accountID, _ := trendData["account_id"].(string)

		var title string
		if group.IsCombined {
			title = fmt.Sprintf("Account: %s (Profiles: %s)", accountID, group.Identifier)
		} else {
			title = fmt.Sprintf("Account: %s (Profile: %s)", accountID, group.Identifier)
		}
		uc.console.Println("\n" + pterm.FgYellow.Sprint(title))

		// Converter para o tipo esperado pela UI
		uiMonthlyCosts := make([]types.MonthlyCost, len(monthlyCosts))
		for i, mc := range monthlyCosts {
			uiMonthlyCosts[i] = types.MonthlyCost{Month: mc.Month, Cost: mc.Cost}
		}
		uc.console.DisplayTrendBars(uiMonthlyCosts)
	}

	return nil
}

func (uc *DashboardUseCase) formatServiceCosts(costs []entity.ServiceCost) []string {
	var formatted []string
	for _, sc := range costs {
		if len(sc.SubCosts) == 0 {
			formatted = append(formatted, fmt.Sprintf("%s: $%.2f", sc.ServiceName, sc.Cost))
			continue
		}

		// Adiciona a linha principal com um indicador de detalhes
		formatted = append(formatted, fmt.Sprintf("%s: $%.2f (details below)", sc.ServiceName, sc.Cost))
		// Adiciona os sub-custos com indentação
		for _, sub := range sc.SubCosts {
			formatted = append(formatted, pterm.FgGray.Sprintf("  └─ %s: $%.2f", sub.ServiceName, sub.Cost))
		}
	}

	if len(formatted) == 0 {
		return []string{"No costs found"}
	}
	return formatted
}

func (uc *DashboardUseCase) formatBudgetInfo(budgets []entity.BudgetInfo) []string {
	var formatted []string
	for _, b := range budgets {
		info := fmt.Sprintf("%s Limit: $%.2f\n%s Actual: $%.2f", b.Name, b.Limit, b.Name, b.Actual)
		if b.Forecast > 0 {
			info += fmt.Sprintf("\n%s Forecast: $%.2f", b.Name, b.Forecast)
		}
		formatted = append(formatted, info)
	}
	if len(formatted) == 0 {
		return []string{"No budgets configured"}
	}
	return formatted
}

func (uc *DashboardUseCase) formatEC2Summary(summary entity.EC2Summary) []string {
	var formatted []string
	states := make([]string, 0, len(summary))
	for state := range summary {
		states = append(states, state)
	}
	sort.Strings(states)

	for _, state := range states {
		if count := summary[state]; count > 0 {
			var coloredState string
			switch state {
			case "running":
				coloredState = pterm.FgGreen.Sprint(state)
			case "stopped":
				coloredState = pterm.FgYellow.Sprint(state)
			default:
				coloredState = pterm.FgCyan.Sprint(state)
			}
			formatted = append(formatted, fmt.Sprintf("%s: %d", coloredState, count))
		}
	}
	if len(formatted) == 0 {
		return []string{"No instances found"}
	}
	return formatted
}

func (uc *DashboardUseCase) calculatePercentageChange(current, previous float64) *float64 {
	if previous > 0.01 {
		change := ((current - previous) / previous) * 100.0
		return &change
	}
	if current < 0.01 {
		zero := 0.0
		return &zero
	}
	return nil
}

func (uc *DashboardUseCase) runS3LifecycleAudit(ctx context.Context, profileGroups []entity.ProfileGroup, args *types.CLIArgs) error {
	uc.console.LogInfo("Auditing S3 lifecycle, encryption and public access...")

	livePrinter, _ := uc.console.GetMultiPrinter().Start()
	defer livePrinter.Stop()

	type row struct {
		Profile   string
		AccountID string
		Audit     entity.S3LifecycleAudit
		Err       error
	}

	results := make([]row, 0, len(profileGroups))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, group := range profileGroups {
		wg.Add(1)
		go func(g entity.ProfileGroup) {
			defer wg.Done()
			bar := uc.console.NewProgressbar(2, fmt.Sprintf("S3 Audit: %s", g.Identifier))
			bar.Start()

			profile := g.Profiles[0]
			accountID, _ := uc.awsRepo.GetAccountID(ctx, profile)
			bar.Increment()

			statuses, err := uc.awsRepo.GetS3LifecycleStatus(ctx, profile)
			if err != nil {
				mu.Lock()
				results = append(results, row{Profile: g.Identifier, Err: err})
				mu.Unlock()
				bar.Increment()
				return
			}
			bar.Increment()

			// Agrega
			total := len(statuses)
			noLifecycle := 0
			versionedMissingNoncurrent := 0
			noIT := 0
			noDefaultEnc := 0
			publicRisk := 0

			regionMap := make(map[string]int)

			sampleNoLifecycle := make([]entity.S3BucketLifecycleStatus, 0, 10)
			sampleVersionedNoNoncurrent := make([]entity.S3BucketLifecycleStatus, 0, 10)
			sampleNoIT := make([]entity.S3BucketLifecycleStatus, 0, 10)
			sampleNoEnc := make([]entity.S3BucketLifecycleStatus, 0, 10)
			samplePublic := make([]entity.S3BucketLifecycleStatus, 0, 10)

			// Ordena por nome para amostras determinísticas
			sort.Slice(statuses, func(i, j int) bool { return statuses[i].Bucket < statuses[j].Bucket })

			for _, s := range statuses {
				// Lifecycle faltando
				if !s.HasLifecycle {
					noLifecycle++
					regionMap[s.Region]++
					if len(sampleNoLifecycle) < 10 {
						sampleNoLifecycle = append(sampleNoLifecycle, s)
					}
				}
				// Versioning sem Noncurrent
				if s.VersioningEnabled && !s.HasNoncurrentLifecycle {
					versionedMissingNoncurrent++
					if len(sampleVersionedNoNoncurrent) < 10 {
						sampleVersionedNoNoncurrent = append(sampleVersionedNoNoncurrent, s)
					}
				}
				// Intelligent-Tiering ausente (nem via Lifecycle, nem config explícita)
				if !s.HasIntelligentTieringCfg && !s.HasIntelligentTieringViaLifecycle {
					noIT++
					if len(sampleNoIT) < 10 {
						sampleNoIT = append(sampleNoIT, s)
					}
				}
				// Criptografia padrão ausente
				if !s.DefaultEncryptionEnabled {
					noDefaultEnc++
					if len(sampleNoEnc) < 10 {
						sampleNoEnc = append(sampleNoEnc, s)
					}
				}
				// Risco de público
				if s.IsPublic || !s.BlockPublicAcls || !s.BlockPublicPolicy || !s.IgnorePublicAcls || !s.RestrictPublicBuckets {
					publicRisk++
					if len(samplePublic) < 10 {
						samplePublic = append(samplePublic, s)
					}
				}
			}

			audit := entity.S3LifecycleAudit{
				Profile:                              g.Identifier,
				AccountID:                            accountID,
				TotalBuckets:                         total,
				NoLifecycleCount:                     noLifecycle,
				VersionedWithoutNoncurrentLifecycle:  versionedMissingNoncurrent,
				NoIntelligentTieringCount:            noIT,
				NoDefaultEncryptionCount:             noDefaultEnc,
				PublicRiskCount:                      publicRisk,
				SampleNoLifecycle:                    sampleNoLifecycle,
				SampleVersionedWithoutNoncurrentRule: sampleVersionedNoNoncurrent,
				SampleNoIntelligentTiering:           sampleNoIT,
				SampleNoDefaultEncryption:            sampleNoEnc,
				SamplePublicRisk:                     samplePublic,
				RegionsNoLifecycle:                   regionMap,
				RecommendedMessage:                   "Set lifecycle (incl. noncurrent rules), enable default encryption (SSE-S3/KMS), enforce Public Access Block and avoid public ACL/policies; consider Intelligent-Tiering for unpredictable access.",
			}

			mu.Lock()
			results = append(results, row{
				Profile:   g.Identifier,
				AccountID: accountID,
				Audit:     audit,
			})
			mu.Unlock()
		}(group)
	}
	wg.Wait()

	// Ordena por perfil
	sort.Slice(results, func(i, j int) bool { return results[i].Profile < results[j].Profile })

	// Monta tabela
	table := uc.console.CreateTable()
	table.AddColumn("Profile")
	table.AddColumn("Account ID")
	table.AddColumn("Buckets")
	table.AddColumn("No Lifecycle")
	table.AddColumn("Versioned w/o Noncurrent")
	table.AddColumn("No Intelligent-Tiering")
	table.AddColumn("No Default Encryption")
	table.AddColumn("Public Risk")
	table.AddColumn("Samples")

	for _, r := range results {
		if r.Err != nil {
			table.AddRow(
				pterm.FgMagenta.Sprint(r.Profile),
				"N/A",
				"N/A",
				pterm.FgRed.Sprintf("Error: %v", r.Err),
				"-",
				"-",
				"-",
				"-",
				"-",
			)
			continue
		}

		// Amostras curtas
		var lines []string
		add := func(prefix string, list []entity.S3BucketLifecycleStatus, max int) {
			limit := len(list)
			if limit > max {
				limit = max
			}
			for i := 0; i < limit; i++ {
				s := list[i]
				lines = append(lines, fmt.Sprintf("[%s] %s (%s)", prefix, s.Bucket, s.Region))
			}
			if len(list) > limit {
				lines = append(lines, fmt.Sprintf("... (+%d more)", len(list)-limit))
			}
		}

		add("NoLifecycle", r.Audit.SampleNoLifecycle, 2)
		add("Versioned-NoNoncurrent", r.Audit.SampleVersionedWithoutNoncurrentRule, 2)
		add("No-IT", r.Audit.SampleNoIntelligentTiering, 2)
		add("No-Enc", r.Audit.SampleNoDefaultEncryption, 2)
		add("Public", r.Audit.SamplePublicRisk, 2)
		if len(lines) == 0 {
			lines = append(lines, "None")
		}

		table.AddRow(
			pterm.FgMagenta.Sprint(r.Profile),
			r.AccountID,
			fmt.Sprintf("%d", r.Audit.TotalBuckets),
			fmt.Sprintf("%d", r.Audit.NoLifecycleCount),
			fmt.Sprintf("%d", r.Audit.VersionedWithoutNoncurrentLifecycle),
			fmt.Sprintf("%d", r.Audit.NoIntelligentTieringCount),
			fmt.Sprintf("%d", r.Audit.NoDefaultEncryptionCount),
			fmt.Sprintf("%d", r.Audit.PublicRiskCount),
			strings.Join(lines, "\n"),
		)
	}
	uc.console.Println("\n" + table.Render())

	// Export
	if args.ReportName != "" {
		uc.console.LogInfo("Exporting S3 lifecycle audit reports...")
		audits := make([]entity.S3LifecycleAudit, 0, len(results))
		for _, r := range results {
			if r.Err == nil {
				audits = append(audits, r.Audit)
			}
		}
		for _, reportType := range args.ReportType {
			switch strings.ToLower(reportType) {
			case "csv":
				path, err := uc.exportRepo.ExportS3LifecycleAuditToCSV(audits, args.ReportName, args.Dir)
				if err != nil {
					uc.console.LogError("Failed to export S3 lifecycle audit CSV: %v", err)
				} else {
					uc.console.LogSuccess("S3 lifecycle audit CSV saved to: %s", path)
				}
			case "json":
				path, err := uc.exportRepo.ExportS3LifecycleAuditToJSON(audits, args.ReportName, args.Dir)
				if err != nil {
					uc.console.LogError("Failed to export S3 lifecycle audit JSON: %v", err)
				} else {
					uc.console.LogSuccess("S3 lifecycle audit JSON saved to: %s", path)
				}
			case "pdf":
				path, err := uc.exportRepo.ExportS3LifecycleAuditToPDF(audits, args.ReportName, args.Dir)
				if err != nil {
					uc.console.LogError("Failed to export S3 lifecycle audit PDF: %v", err)
				} else {
					uc.console.LogSuccess("S3 lifecycle audit PDF saved to: %s", path)
				}
			}
		}
	}

	return nil
}

func (uc *DashboardUseCase) runCommitmentsReport(ctx context.Context, profileGroups []entity.ProfileGroup, args *types.CLIArgs) error {
	uc.console.LogInfo("Analysing Savings Plans / Reserved Instances coverage & utilization...")

	livePrinter, _ := uc.console.GetMultiPrinter().Start()
	defer livePrinter.Stop()

	type row struct {
		Profile string
		Report  entity.CommitmentsReport
		Err     error
	}
	results := make([]row, 0, len(profileGroups))

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, group := range profileGroups {
		wg.Add(1)
		go func(g entity.ProfileGroup) {
			defer wg.Done()
			bar := uc.console.NewProgressbar(2, fmt.Sprintf("Commitments: %s", g.Identifier))
			bar.Start()

			profile := g.Profiles[0]

			var timeRange *int
			if args.TimeRange != nil && *args.TimeRange > 0 {
				timeRange = args.TimeRange
			}

			sp, err1 := uc.awsRepo.GetSavingsPlansSummary(ctx, profile, timeRange, args.Tag)
			bar.Increment()
			ri, err2 := uc.awsRepo.GetReservationSummary(ctx, profile, timeRange, args.Tag)
			bar.Increment()

			if err1 != nil {
				mu.Lock()
				results = append(results, row{
					Profile: g.Identifier,
					Err:     fmt.Errorf("SP error: %w", err1),
				})
				mu.Unlock()
				return
			}
			if err2 != nil {
				mu.Lock()
				results = append(results, row{
					Profile: g.Identifier,
					Err:     fmt.Errorf("RI error: %w", err2),
				})
				mu.Unlock()
				return
			}

			rep := entity.CommitmentsReport{
				AccountID:  sp.AccountID,
				Profile:    g.Identifier,
				SPSummary:  sp,
				RISummary:  ri,
				PeriodName: sp.PeriodName,
			}

			mu.Lock()
			results = append(results, row{
				Profile: g.Identifier,
				Report:  rep,
			})
			mu.Unlock()
		}(group)
	}
	wg.Wait()

	sort.Slice(results, func(i, j int) bool { return results[i].Profile < results[j].Profile })

	// Monta tabela
	table := uc.console.CreateTable()
	table.AddColumn("Profile")
	table.AddColumn("Account ID")
	table.AddColumn("Period")
	table.AddColumn("SP Coverage %")
	table.AddColumn("SP Util %")
	table.AddColumn("SP Unused ($)")
	table.AddColumn("RI Coverage %")
	table.AddColumn("RI Util %")
	table.AddColumn("RI Unused (hrs)")

	for _, r := range results {
		if r.Err != nil {
			table.AddRow(
				pterm.FgMagenta.Sprint(r.Profile),
				"N/A",
				"N/A",
				pterm.FgRed.Sprintf("Error: %v", r.Err),
				"-", "-", "-", "-", "-",
			)
			continue
		}
		rep := r.Report
		period := fmt.Sprintf("%s to %s", rep.SPSummary.PeriodStart.Format("2006-01-02"), rep.SPSummary.PeriodEnd.Format("2006-01-02"))

		spCoverage := fmt.Sprintf("%.2f%%", rep.SPSummary.CoveragePercent)
		spUtil := fmt.Sprintf("%.2f%%", rep.SPSummary.UtilizationPercent)
		spUnused := fmt.Sprintf("$%.2f", rep.SPSummary.UnusedCommitment)
		if rep.SPSummary.DataUnavailable {
			spCoverage = "Data Unavailable"
			spUtil = "Data Unavailable"
			spUnused = "Data Unavailable"
		}

		riCoverage := fmt.Sprintf("%.2f%%", rep.RISummary.CoveragePercent)
		riUtil := fmt.Sprintf("%.2f%%", rep.RISummary.UtilizationPercent)
		riUnused := fmt.Sprintf("%.2f", rep.RISummary.UnusedHours)
		if rep.RISummary.DataUnavailable {
			riCoverage = "Data Unavailable"
			riUtil = "Data Unavailable"
			riUnused = "Data Unavailable"
		}

		table.AddRow(
			pterm.FgMagenta.Sprint(rep.Profile),
			rep.AccountID,
			period,
			spCoverage,
			spUtil,
			spUnused,
			riCoverage,
			riUtil,
			riUnused,
		)
	}
	uc.console.Println("\n" + table.Render())

	// Exibir Top “gaps” por serviço para SP e RI (opcional curto)
	const maxLines = 5
	for _, r := range results {
		if r.Err != nil {
			continue
		}
		rep := r.Report
		uc.console.Println(pterm.FgYellow.Sprintf("\nTop SP On-Demand by Service — %s", r.Profile))
		spList := rep.SPSummary.PerServiceCoverage
		if len(spList) == 0 {
			uc.console.Println("  None")
		} else {
			limit := len(spList)
			if limit > maxLines {
				limit = maxLines
			}
			for i := 0; i < limit; i++ {
				l := spList[i]
				uc.console.Println(fmt.Sprintf("  - %s: coverage %.2f%%, OnDemand $%.2f", l.Service, l.CoveragePercent, l.OnDemandCost))
			}
			if len(spList) > limit {
				uc.console.Println(fmt.Sprintf("  ... (+%d more)", len(spList)-limit))
			}
		}

		uc.console.Println(pterm.FgYellow.Sprintf("\nTop RI On-Demand by Service — %s", r.Profile))
		riList := rep.RISummary.PerServiceCoverage
		if len(riList) == 0 {
			uc.console.Println("  None")
		} else {
			limit := len(riList)
			if limit > maxLines {
				limit = maxLines
			}
			for i := 0; i < limit; i++ {
				l := riList[i]
				uc.console.Println(fmt.Sprintf("  - %s: coverage %.2f%%, OnDemand Hrs %.2f", l.Service, l.CoveragePercent, l.OnDemandCost))
			}
			if len(riList) > limit {
				uc.console.Println(fmt.Sprintf("  ... (+%d more)", len(riList)-limit))
			}
		}
	}

	// Export
	if args.ReportName != "" {
		uc.console.LogInfo("Exporting commitments reports...")
		reports := make([]entity.CommitmentsReport, 0, len(results))
		for _, r := range results {
			if r.Err == nil {
				reports = append(reports, r.Report)
			}
		}
		for _, reportType := range args.ReportType {
			switch strings.ToLower(reportType) {
			case "csv":
				path, err := uc.exportRepo.ExportCommitmentsReportToCSV(reports, args.ReportName, args.Dir)
				if err != nil {
					uc.console.LogError("Failed to export commitments CSV: %v", err)
				} else {
					uc.console.LogSuccess("Commitments CSV saved to: %s", path)
				}
			case "json":
				path, err := uc.exportRepo.ExportCommitmentsReportToJSON(reports, args.ReportName, args.Dir)
				if err != nil {
					uc.console.LogError("Failed to export commitments JSON: %v", err)
				} else {
					uc.console.LogSuccess("Commitments JSON saved to: %s", path)
				}
			case "pdf":
				path, err := uc.exportRepo.ExportCommitmentsReportToPDF(reports, args.ReportName, args.Dir)
				if err != nil {
					uc.console.LogError("Failed to export commitments PDF: %v", err)
				} else {
					uc.console.LogSuccess("Commitments PDF saved to: %s", path)
				}
			}
		}
	}

	return nil
}

func (uc *DashboardUseCase) runFullAuditReport(ctx context.Context, profileGroups []entity.ProfileGroup, args *types.CLIArgs) error {
	uc.console.LogInfo("Running Full Audit...")

	livePrinter, _ := uc.console.GetMultiPrinter().Start()
	defer livePrinter.Stop()

	type row struct {
		Profile string
		Report  entity.FullAuditReport
		Err     error
	}
	results := make([]row, 0, len(profileGroups))

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, group := range profileGroups {
		wg.Add(1)
		go func(g entity.ProfileGroup) {
			defer wg.Done()
			const totalSteps = 6 // Main Audit, Transfer, Logs, S3, SP, RI
			bar := uc.console.NewProgressbar(totalSteps, fmt.Sprintf("Full Audit: %s", g.Identifier))
			bar.Start()

			profile := g.Profiles[0]
			accountID, _ := uc.awsRepo.GetAccountID(ctx, profile)
			report := entity.FullAuditReport{Profile: g.Identifier, AccountID: accountID}

			var innerWg sync.WaitGroup
			innerWg.Add(totalSteps)

			// 1. Main Audit
			go func() {
				defer innerWg.Done()
				defer bar.Increment()
				regions := args.Regions
				if len(regions) == 0 {
					regions, _ = uc.awsRepo.GetAccessibleRegions(ctx, profile)
				}
				natCosts, _ := uc.awsRepo.GetNatGatewayCost(ctx, profile, args.TimeRange, args.Tag)
				idleLBs, _ := uc.awsRepo.GetIdleLoadBalancers(ctx, profile, regions)
				stopped, _ := uc.awsRepo.GetStoppedInstances(ctx, profile, regions)
				unusedVols, _ := uc.awsRepo.GetUnusedVolumes(ctx, profile, regions)
				unusedEIPs, _ := uc.awsRepo.GetUnusedEIPs(ctx, profile, regions)
				untagged, _ := uc.awsRepo.GetUntaggedResources(ctx, profile, regions)
				unusedEndpoints, _ := uc.awsRepo.GetUnusedVpcEndpoints(ctx, profile, regions)
				budgets, _ := uc.awsRepo.GetBudgets(ctx, profile)

				report.MainAudit = &entity.AuditData{
					Profile:            g.Identifier,
					AccountID:          accountID,
					NatGatewayCosts:    formatNatGatewayCosts(natCosts),
					IdleLoadBalancers:  formatAuditMap(idleLBs, "Idle Load Balancers"),
					StoppedInstances:   formatAuditMap(stopped, "Stopped Instances"),
					UnusedVolumes:      formatAuditMap(unusedVols, "Unused Volumes"),
					UnusedEIPs:         formatAuditMap(unusedEIPs, "Unused Elastic IPs"),
					UntaggedResources:  formatAuditMapForUntagged(untagged),
					UnusedVpcEndpoints: formatAuditMap(unusedEndpoints, "Unused VPC Endpoints"),
					BudgetAlerts:       formatBudgetAlerts(budgets),
				}
			}()

			// 2. Transfer Audit
			go func() {
				defer innerWg.Done()
				defer bar.Increment()
				if transfer, err := uc.awsRepo.GetDataTransferBreakdown(ctx, profile, args.TimeRange, args.Tag); err == nil {
					report.TransferAudit = &transfer
				}
			}()

			// 3. Logs Audit
			go func() {
				defer innerWg.Done()
				defer bar.Increment()
				regions := args.Regions
				if len(regions) == 0 {
					regions, _ = uc.awsRepo.GetAccessibleRegions(ctx, profile)
				}
				if logGroups, err := uc.awsRepo.GetCloudWatchLogGroups(ctx, profile, regions); err == nil {
					noRetention := make([]entity.CloudWatchLogGroupInfo, 0)
					var totalBytes int64
					for _, lg := range logGroups {
						totalBytes += lg.StoredBytes
						if lg.RetentionDays == 0 {
							noRetention = append(noRetention, lg)
						}
					}
					sort.Slice(noRetention, func(i, j int) bool { return noRetention[i].StoredBytes > noRetention[j].StoredBytes })
					top := noRetention
					if len(noRetention) > 10 {
						top = noRetention[:10]
					}

					report.LogsAudit = &entity.CloudWatchLogsAudit{
						Profile:            g.Identifier,
						AccountID:          accountID,
						NoRetentionCount:   len(noRetention),
						NoRetentionTopN:    top,
						TotalStoredGB:      float64(totalBytes) / (1024.0 * 1024.0 * 1024.0),
						RecommendedMessage: "Set retention days to avoid unlimited storage growth.",
					}
				}
			}()

			// 4. S3 Audit
			go func() {
				defer innerWg.Done()
				defer bar.Increment()
				if statuses, err := uc.awsRepo.GetS3LifecycleStatus(ctx, profile); err == nil {
					total := len(statuses)
					noLifecycle, versionedMissingNoncurrent, noIT, noDefaultEnc, publicRisk := 0, 0, 0, 0, 0
					regionMap := make(map[string]int)
					sampleNoLifecycle, sampleVersionedNoNoncurrent, sampleNoIT, sampleNoEnc, samplePublic := make([]entity.S3BucketLifecycleStatus, 0, 10), make([]entity.S3BucketLifecycleStatus, 0, 10), make([]entity.S3BucketLifecycleStatus, 0, 10), make([]entity.S3BucketLifecycleStatus, 0, 10), make([]entity.S3BucketLifecycleStatus, 0, 10)
					sort.Slice(statuses, func(i, j int) bool { return statuses[i].Bucket < statuses[j].Bucket })

					for _, s := range statuses {
						if !s.HasLifecycle {
							noLifecycle++
							regionMap[s.Region]++
							if len(sampleNoLifecycle) < 10 {
								sampleNoLifecycle = append(sampleNoLifecycle, s)
							}
						}
						if s.VersioningEnabled && !s.HasNoncurrentLifecycle {
							versionedMissingNoncurrent++
							if len(sampleVersionedNoNoncurrent) < 10 {
								sampleVersionedNoNoncurrent = append(sampleVersionedNoNoncurrent, s)
							}
						}
						if !s.HasIntelligentTieringCfg && !s.HasIntelligentTieringViaLifecycle {
							noIT++
							if len(sampleNoIT) < 10 {
								sampleNoIT = append(sampleNoIT, s)
							}
						}
						if !s.DefaultEncryptionEnabled {
							noDefaultEnc++
							if len(sampleNoEnc) < 10 {
								sampleNoEnc = append(sampleNoEnc, s)
							}
						}
						if s.IsPublic || !s.BlockPublicAcls || !s.BlockPublicPolicy || !s.IgnorePublicAcls || !s.RestrictPublicBuckets {
							publicRisk++
							if len(samplePublic) < 10 {
								samplePublic = append(samplePublic, s)
							}
						}
					}
					report.S3Audit = &entity.S3LifecycleAudit{
						Profile:                              g.Identifier,
						AccountID:                            accountID,
						TotalBuckets:                         total,
						NoLifecycleCount:                     noLifecycle,
						VersionedWithoutNoncurrentLifecycle:  versionedMissingNoncurrent,
						NoIntelligentTieringCount:            noIT,
						NoDefaultEncryptionCount:             noDefaultEnc,
						PublicRiskCount:                      publicRisk,
						SampleNoLifecycle:                    sampleNoLifecycle,
						SampleVersionedWithoutNoncurrentRule: sampleVersionedNoNoncurrent,
						SampleNoIntelligentTiering:           sampleNoIT,
						SampleNoDefaultEncryption:            sampleNoEnc,
						SamplePublicRisk:                     samplePublic,
						RegionsNoLifecycle:                   regionMap,
						RecommendedMessage:                   "Set lifecycle (incl. noncurrent rules), enable default encryption (SSE-S3/KMS), enforce Public Access Block and avoid public ACL/policies; consider Intelligent-Tiering for unpredictable access.",
					}
				}
			}()

			// 5. Commitments (SP)
			go func() {
				defer innerWg.Done()
				defer bar.Increment()
				if sp, err := uc.awsRepo.GetSavingsPlansSummary(ctx, profile, args.TimeRange, args.Tag); err == nil {
					mu.Lock()
					if report.CommitmentsAudit == nil {
						report.CommitmentsAudit = &entity.CommitmentsReport{}
					}
					report.CommitmentsAudit.SPSummary = sp
					mu.Unlock()
				}
			}()

			// 6. Commitments (RI)
			go func() {
				defer innerWg.Done()
				defer bar.Increment()
				if ri, err := uc.awsRepo.GetReservationSummary(ctx, profile, args.TimeRange, args.Tag); err == nil {
					mu.Lock()
					if report.CommitmentsAudit == nil {
						report.CommitmentsAudit = &entity.CommitmentsReport{}
					}
					report.CommitmentsAudit.RISummary = ri
					mu.Unlock()
				}
			}()

			innerWg.Wait()

			// Finaliza o CommitmentsReport
			if report.CommitmentsAudit != nil {
				report.CommitmentsAudit.Profile = g.Identifier
				report.CommitmentsAudit.AccountID = accountID
				report.CommitmentsAudit.PeriodName = report.CommitmentsAudit.SPSummary.PeriodName
			}

			mu.Lock()
			results = append(results, row{Profile: g.Identifier, Report: report})
			mu.Unlock()
		}(group)
	}
	wg.Wait()

	// Ordena os resultados por perfil
	sort.Slice(results, func(i, j int) bool { return results[i].Profile < results[j].Profile })

	// Exibe um resumo no terminal
	uc.console.Println("\n" + pterm.DefaultSection.WithLevel(1).Sprint("Full Audit Summary"))
	for _, r := range results {
		if r.Err != nil {
			uc.console.Println(pterm.FgRed.Sprintf("Error auditing profile %s: %v", r.Profile, r.Err))
			continue
		}
		rep := r.Report
		uc.console.Println(pterm.FgMagenta.Sprintf("\nProfile: %s (Account: %s)", rep.Profile, rep.AccountID))

		// Resumo de cada sub-relatório
		if a := rep.MainAudit; a != nil {
			var findings []string
			if a.NatGatewayCosts != "None" {
				findings = append(findings, "High-Cost NATs")
			}
			if a.UnusedVolumes != "None" {
				findings = append(findings, "Unused Volumes")
			}
			if a.UntaggedResources != "None" {
				findings = append(findings, "Untagged Resources")
			}
			uc.console.Println(fmt.Sprintf("  - Main Audit: %s", strings.Join(findings, ", ")))
		}
		if t := rep.TransferAudit; t != nil {
			uc.console.Println(fmt.Sprintf("  - Data Transfer: Total $%.2f", t.Total))
		}
		if l := rep.LogsAudit; l != nil {
			uc.console.Println(fmt.Sprintf("  - CloudWatch Logs: %d groups with no retention", l.NoRetentionCount))
		}
		if s := rep.S3Audit; s != nil {
			uc.console.Println(fmt.Sprintf("  - S3 Buckets: %d with no lifecycle, %d with public risk", s.NoLifecycleCount, s.PublicRiskCount))
		}
		if c := rep.CommitmentsAudit; c != nil {
			uc.console.Println(fmt.Sprintf("  - Commitments: SP Coverage %.2f%%, RI Coverage %.2f%%", c.SPSummary.CoveragePercent, c.RISummary.CoveragePercent))
		}
	}

	// Exporta os relatórios
	if args.ReportName != "" {
		uc.console.LogInfo("Exporting full audit reports...")

		fullReports := make([]entity.FullAuditReport, 0, len(results))
		for _, r := range results {
			if r.Err == nil {
				fullReports = append(fullReports, r.Report)
			}
		}

		for _, reportType := range args.ReportType {
			switch strings.ToLower(reportType) {
			case "csv":
				paths, err := uc.exportRepo.ExportFullAuditReportToCSV(fullReports, args.ReportName, args.Dir)
				if err != nil {
					uc.console.LogError("Failed to export full audit to CSV package: %v", err)
				} else {
					uc.console.LogSuccess("Full audit CSV package saved to: %s", strings.Join(paths, ", "))
				}
			case "json":
				path, err := uc.exportRepo.ExportFullAuditReportToJSON(fullReports, args.ReportName, args.Dir)
				if err != nil {
					uc.console.LogError("Failed to export full audit to JSON: %v", err)
				} else {
					uc.console.LogSuccess("Full audit JSON report saved to: %s", path)
				}
			case "pdf":
				path, err := uc.exportRepo.ExportFullAuditReportToPDF(fullReports, args.ReportName, args.Dir)
				if err != nil {
					uc.console.LogError("Failed to export full audit to PDF: %v", err)
				} else {
					uc.console.LogSuccess("Full audit PDF report saved to: %s", path)
				}
			}
		}
	}

	return nil
}

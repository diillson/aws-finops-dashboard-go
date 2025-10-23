package cli

import (
	"context"
	"os"
	"path/filepath"

	"github.com/diillson/aws-finops-dashboard-go/pkg/version"

	"github.com/diillson/aws-finops-dashboard-go/internal/application/usecase"
	"github.com/diillson/aws-finops-dashboard-go/internal/shared/types"
	"github.com/spf13/cobra"
)

// CLIApp represents the command-line interface application.
type CLIApp struct {
	rootCmd          *cobra.Command
	dashboardUseCase *usecase.DashboardUseCase
	version          string
}

// NewCLIApp cria uma nova aplicação CLI.
func NewCLIApp(versionStr string) *CLIApp {
	app := &CLIApp{
		version: versionStr,
	}

	// Obtem a versão formatada
	formattedVersion := version.FormatVersion()

	rootCmd := &cobra.Command{
		Use:     "aws-finops",
		Short:   "AWS FinOps Dashboard CLI",
		Version: formattedVersion, // Use a versão formatada
		RunE:    app.runCommand,
	}

	// Personaliza a template para incluir mais informações de versão
	rootCmd.SetVersionTemplate(`{{printf "AWS FinOps Dashboard version: %s\n" .Version}}`)

	// Adiciona flags de linha de comando
	rootCmd.PersistentFlags().StringP("config-file", "C", "", "Path to a TOML, YAML, or JSON configuration file")
	rootCmd.PersistentFlags().StringSliceP("profiles", "p", nil, "Specific AWS profiles to use (comma-separated)")
	rootCmd.PersistentFlags().StringSliceP("regions", "r", nil, "AWS regions to check for EC2 instances (comma-separated)")
	rootCmd.PersistentFlags().BoolP("all", "a", false, "Use all available AWS profiles")
	rootCmd.PersistentFlags().BoolP("combine", "c", false, "Combine profiles from the same AWS account")
	rootCmd.PersistentFlags().StringP("report-name", "n", "", "Specify the base name for the report file (without extension)")
	rootCmd.PersistentFlags().StringSliceP("report-type", "y", []string{"csv"}, "Specify report types: csv, json, pdf")
	rootCmd.PersistentFlags().StringP("dir", "d", "", "Directory to save the report files (default: current directory)")
	rootCmd.PersistentFlags().IntP("time-range", "t", 0, "Time range for cost data in days (default: current month)")
	rootCmd.PersistentFlags().StringSliceP("tag", "g", nil, "Cost allocation tag to filter resources, e.g., --tag Team=DevOps")
	rootCmd.PersistentFlags().Bool("trend", false, "Display a trend report as bars for the past 6 months time range")
	rootCmd.PersistentFlags().Bool("audit", false, "Display an audit report with cost anomalies, stopped EC2 instances, unused EBS volumes, budget alerts, and more")
	rootCmd.PersistentFlags().Bool("breakdown-costs", false, "Show a detailed cost breakdown for services like Data Transfer.")

	app.rootCmd = rootCmd
	return app
}

// Execute runs the CLI application.
func (app *CLIApp) Execute() error {
	return app.rootCmd.Execute()
}

// parseArgs parses command-line arguments into a CLIArgs struct.
func (app *CLIApp) parseArgs() (*types.CLIArgs, error) {
	configFile, _ := app.rootCmd.Flags().GetString("config-file")
	profiles, _ := app.rootCmd.Flags().GetStringSlice("profiles")
	regions, _ := app.rootCmd.Flags().GetStringSlice("regions")
	all, _ := app.rootCmd.Flags().GetBool("all")
	combine, _ := app.rootCmd.Flags().GetBool("combine")
	reportName, _ := app.rootCmd.Flags().GetString("report-name")
	reportType, _ := app.rootCmd.Flags().GetStringSlice("report-type")
	dir, _ := app.rootCmd.Flags().GetString("dir")
	timeRange, _ := app.rootCmd.Flags().GetInt("time-range")
	tag, _ := app.rootCmd.Flags().GetStringSlice("tag")
	trend, _ := app.rootCmd.Flags().GetBool("trend")
	audit, _ := app.rootCmd.Flags().GetBool("audit")
	breakdownCosts, _ := app.rootCmd.Flags().GetBool("breakdown-costs")

	// Set default directory to current working directory if not specified
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		dir = cwd
	} else {
		// Convert to absolute path
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		dir = absDir
	}

	timeRangePtr := &timeRange
	if timeRange == 0 {
		timeRangePtr = nil
	}

	args := &types.CLIArgs{
		ConfigFile:     configFile,
		Profiles:       profiles,
		Regions:        regions,
		All:            all,
		Combine:        combine,
		ReportName:     reportName,
		ReportType:     reportType,
		Dir:            dir,
		TimeRange:      timeRangePtr,
		Tag:            tag,
		Trend:          trend,
		Audit:          audit,
		BreakdownCosts: breakdownCosts,
	}

	return args, nil
}

// runCommand é o ponto de entrada principal para o comando CLI.
func (app *CLIApp) runCommand(cmd *cobra.Command, args []string) error {
	// Exibe o banner de boas-vindas
	displayWelcomeBanner(app.version)

	// Verifica a versão mais recente disponível
	go version.CheckLatestVersion(app.version)

	// Analisa os argumentos da linha de comando
	cliArgs, err := app.parseArgs()
	if err != nil {
		return err
	}

	// Lida com o arquivo de configuração, se especificado
	if cliArgs.ConfigFile != "" {
		// Carrega e mescla a configuração
		// Isso será implementado pelo repositório de configuração
	}

	// Executa o dashboard
	ctx := context.Background()
	return app.dashboardUseCase.RunDashboard(ctx, cliArgs)
}

// SetDashboardUseCase sets the dashboard use case for the CLI app.
func (app *CLIApp) SetDashboardUseCase(useCase *usecase.DashboardUseCase) {
	app.dashboardUseCase = useCase
}

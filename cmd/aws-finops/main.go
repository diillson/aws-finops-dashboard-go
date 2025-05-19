package main

import (
	"fmt"
	"os"

	"github.com/diillson/aws-finops-dashboard-go/internal/adapter/driven/aws"
	"github.com/diillson/aws-finops-dashboard-go/internal/adapter/driven/config"
	"github.com/diillson/aws-finops-dashboard-go/internal/adapter/driven/export"
	"github.com/diillson/aws-finops-dashboard-go/internal/adapter/driving/cli"
	"github.com/diillson/aws-finops-dashboard-go/internal/application/usecase"
	"github.com/diillson/aws-finops-dashboard-go/pkg/console"
	"github.com/diillson/aws-finops-dashboard-go/pkg/version"
)

func main() {
	// Inicializa o aplicativo CLI
	app := cli.NewCLIApp(version.Version)

	// Inicializa os repositórios
	awsRepo := aws.NewAWSRepository()
	exportRepo := export.NewExportRepository()
	configRepo := config.NewConfigRepository()
	consoleImpl := console.NewConsole()

	// Inicializa o caso de uso
	dashboardUseCase := usecase.NewDashboardUseCase(
		awsRepo,
		exportRepo,
		configRepo,
		consoleImpl,
	)

	// Define o caso de uso no aplicativo CLI
	app.SetDashboardUseCase(dashboardUseCase)

	// Executa o aplicativo
	if err := app.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

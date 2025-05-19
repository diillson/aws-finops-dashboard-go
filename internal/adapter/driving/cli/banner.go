package cli

import (
	"fmt"

	"github.com/diillson/aws-finops-dashboard-go/pkg/version"
	"github.com/fatih/color"
)

// displayWelcomeBanner exibe o banner de boas-vindas com informações de versão.
func displayWelcomeBanner(versionStr string) {
	banner := `
          /$$$$$$  /$$      /$$  /$$$$$$        /$$$$$$$$ /$$            /$$$$$$                     
         /$$__  $$| $$  /$ | $$ /$$__  $$      | $$_____/|__/           /$$__  $$                    
        | $$  \ $$| $$ /$$$| $$| $$  \__/      | $$       /$$ /$$$$$$$ | $$  \ $$  /$$$$$$   /$$$$$$$
        | $$$$$$$$| $$/$$ $$ $$|  $$$$$$       | $$$$$   | $$| $$__  $$| $$  | $$ /$$__  $$ /$$_____/
        | $$__  $$| $$$$_  $$$$ \____  $$      | $$__/   | $$| $$  \ $$| $$  | $$| $$  \ $$|  $$$$$$ 
        | $$  | $$| $$$/ \  $$$ /$$  \ $$      | $$      | $$| $$  | $$| $$  | $$| $$  | $$ \____  $$
        | $$  | $$| $$/   \  $$|  $$$$$$/      | $$      | $$| $$  | $$|  $$$$$$/| $$$$$$$/ /$$$$$$$/
        |__/  |__/|__/     \__/ \______/       |__/      |__/|__/  |__/ \______/ | $$____/ |_______/ 
                                                                                 | $$                
                                                                                 | $$                
                                                                                 |__/                
        `
	red := color.New(color.FgRed, color.Bold).SprintFunc()
	blue := color.New(color.FgBlue, color.Bold).SprintFunc()

	fmt.Println(red(banner))

	// Obtem a string formatada da versão através do pacote version
	formattedVersion := version.FormatVersion()
	fmt.Println(blue(fmt.Sprintf("AWS FinOps Dashboard CLI (v%s)", formattedVersion)))
}

// checkLatestVersion verifica se uma versão mais recente está disponível.
func checkLatestVersion(currentVersion string) {
	// Usa a função do pacote version para verificar por atualizações
	version.CheckLatestVersion(currentVersion)
}

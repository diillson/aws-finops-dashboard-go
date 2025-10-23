# AWS FinOps Dashboard (Go) — CLI

Uma CLI para visualizar e auditar custos na AWS (FinOps), com suporte a múltiplos perfis, combinação por conta, exportação de relatórios (CSV/JSON/PDF), análise de tendências e auditoria de otimizações (NAT Gateways caros, LBs ociosos, Volumes/EIPs sem uso, recursos sem tags etc.).

---

## Sumário
- [Recursos (Features)](#recursos-features)
- [Pré-requisitos](#pré-requisitos)
- [Instalação e Build](#instalação-e-build)
  - [Build com versão correta (ldflags)](#build-com-versão-correta-ldflags)
  - [Makefile (opcional)](#makefile-opcional)
- [Uso rápido](#uso-rápido)
- [Flags da CLI](#flags-da-cli)
- [Arquivo de configuração (TOML/YAML/JSON)](#arquivo-de-configuração-tomlyamljson)
  - [Estrutura](#estrutura)
  - [Exemplos](#exemplos)
  - [Como usar e precedência](#como-usar-e-precedência)
- [Casos de Uso (Exemplos práticos)](#casos-de-uso-exemplos-práticos)
- [Relatórios e Exportação](#relatórios-e-exportação)
- [Fluxo interno e Arquitetura](#fluxo-interno-e-arquitetura)
- [Permissões AWS necessárias](#permissões-aws-necessárias)
- [Solução de problemas (Troubleshooting)](#solução-de-problemas-troubleshooting)
- [Observações de desempenho](#observações-de-desempenho)
- [Segurança](#segurança)
- [Screenshots](#screenshots)
- [Licença e Créditos](#licença-e-créditos)

---

## Recursos (Features)

- Dashboard de custos por perfil/conta com:
  - Custo do período anterior vs atual e variação percentual
  - Custos por serviço, com detalhamento opcional (usage-type) para serviços como Data Transfer, EC2-Other e VPC
  - Sumário de instâncias EC2 por estado
  - Status de Budgets (limite, atual, forecast)
- Combinação por conta com `--combine`
- Filtros por tags de alocação (`--tag`)
- Período personalizável (`--time-range`)
- Análise de tendências (últimos 6 meses) com `--trend`
- Auditoria de otimização (`--audit`):
  - NAT Gateways com alto custo
  - Load Balancers ociosos
  - Volumes EBS e EIPs sem uso
  - EC2 paradas
  - Recursos sem tags (EC2, RDS, Lambda)
  - VPC Endpoints (Interface) sem uso
  - Alertas de Budget
- Exportação: CSV, JSON e PDF
- Configurações via TOML, YAML ou JSON
- Verificação de atualização via GitHub Releases
- Interface rica no terminal (pterm): banner, progress bar, tabelas etc.

---

## Pré-requisitos

- Go **1.24+** (recomendado)
- AWS CLI configurado com credenciais válidas
- **Cost Explorer** habilitado
- Permissões de IAM adequadas ([ver abaixo](#permissões-aws-necessárias))

---

## Instalação e Build

Clone o repositório:

```bash
git clone https://github.com/diillson/aws-finops-dashboard-go.git
cd aws-finops-dashboard-go
````

Build básico:

```bash
go build -o bin/aws-finops ./cmd/aws-finops
```

Executável:

```bash
./bin/aws-finops --help
```

### Build com versão correta (ldflags)

Linux/macOS:

```bash
go build -ldflags "-s -w \
  -X github.com/diillson/aws-finops-dashboard-go/pkg/version.Version=1.2.0 \
  -X github.com/diillson/aws-finops-dashboard-go/pkg/version.Commit=$(git rev-parse --short HEAD) \
  -X github.com/diillson/aws-finops-dashboard-go/pkg/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o bin/aws-finops ./cmd/aws-finops
```

Windows (PowerShell):

```powershell
$commit = git rev-parse --short HEAD
$buildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
go build -ldflags "-s -w `
  -X github.com/diillson/aws-finops-dashboard-go/pkg/version.Version=1.2.3 `
  -X github.com/diillson/aws-finops-dashboard-go/pkg/version.Commit=$commit `
  -X github.com/diillson/aws-finops-dashboard-go/pkg/version.BuildTime=$buildTime" `
  -o bin/aws-finops ./cmd/aws-finops
```

Ao iniciar, verá algo como:

```
AWS FinOps Dashboard CLI (v1.2.0 (commit: abc1234, built at: 2025-10-23T10:20:30Z))
```

Sem `-ldflags`, o padrão é:

```
AWS FinOps Dashboard CLI (v0.0.0-dev (development))
```

---

### Makefile (opcional)

O projeto inclui um Makefile para simplificar o build.

Uso:

```bash
make build
./bin/aws-finops --version
```

---

## Uso rápido

```bash
aws-finops --help
aws-finops --version
aws-finops -p my-prod
aws-finops --all
aws-finops --all --combine
aws-finops -p prod -g Environment=Production -n finops-report -y csv -y pdf -d ./reports
aws-finops -p prod --trend
aws-finops --all --audit -n audit-$(date +%Y%m%d) -y pdf -d ./audits
```

---

## Flags da CLI

```
-C, --config-file string   Caminho do arquivo de configuração
-p, --profiles strings     Perfis AWS (separados por vírgula)
-r, --regions strings      Regiões AWS
-a, --all                  Usa todos os perfis disponíveis
-c, --combine              Combina perfis da mesma conta
-n, --report-name string   Nome base do relatório
-y, --report-type strings  Tipos: csv, json, pdf
-d, --dir string           Diretório de saída
-t, --time-range int       Intervalo em dias (padrão: mês corrente)
-g, --tag strings          Filtro por tag (ex: Team=DevOps)
--trend                    Análise de tendência (6 meses)
--audit                    Auditoria de otimização
--breakdown-costs          Detalhamento de custos (usage-type)
--version                  Mostra a versão
--help                     Ajuda
```

---

## Arquivo de configuração (TOML/YAML/JSON)

### Estrutura

```go
type Config struct {
  Profiles   []string `json:"profiles" yaml:"profiles" toml:"profiles"`
  Regions    []string `json:"regions" yaml:"regions" toml:"regions"`
  Combine    bool     `json:"combine" yaml:"combine" toml:"combine"`
  ReportName string   `json:"report_name" yaml:"report_name" toml:"report_name"`
  ReportType []string `json:"report_type" yaml:"report_type" toml:"report_type"`
  Dir        string   `json:"dir" yaml:"dir" toml:"dir"`
  TimeRange  int      `json:"time_range" yaml:"time_range" toml:"time_range"`
  Tag        []string `json:"tag" yaml:"tag" toml:"tag"`
  Audit      bool     `json:"audit" yaml:"audit" toml:"audit"`
  Trend      bool     `json:"trend" yaml:"trend" toml:"trend"`
  All        bool     `json:"all" yaml:"all" toml:"all"`
}
```

### Exemplos

**TOML**

```toml
profiles = ["production", "development", "data-warehouse"]
regions = ["us-east-1", "us-west-2", "eu-central-1"]
combine = true
report_name = "aws-finops-monthly"
report_type = ["csv", "pdf"]
dir = "/home/user/reports/aws"
time_range = 30
tag = ["Environment=Production", "Department=IT"]
audit = false
trend = false
```

**YAML**

```yaml
profiles:
  - production
  - development
  - data-warehouse
regions:
  - us-east-1
  - us-west-2
  - eu-central-1
combine: true
report_name: aws-finops-monthly
report_type: [csv, pdf]
dir: /home/user/reports/aws
time_range: 30
tag: ["Environment=Production", "Department=IT"]
audit: false
trend: false
```

**JSON**

```json
{
  "profiles": ["production", "development", "data-warehouse"],
  "regions": ["us-east-1", "us-west-2", "eu-central-1"],
  "combine": true,
  "report_name": "aws-finops-monthly",
  "report_type": ["csv", "pdf"],
  "dir": "/home/user/reports/aws",
  "time_range": 30,
  "tag": ["Environment=Production", "Department=IT"],
  "audit": false,
  "trend": false
}
```

### Como usar e precedência

```bash
aws-finops --config-file /path/config.yaml
```

Flags de linha de comando **sobrescrevem** as do arquivo.

---

## Casos de Uso (Exemplos práticos)

```bash
aws-finops --all --combine -n monthly-costs -y csv -y pdf -d ./reports
aws-finops -p prod -p staging --audit -r us-east-1 -r eu-west-1 -n audit-jan -y pdf -d ./audits
aws-finops -p prod --trend -g Department=Engineering -t 180
aws-finops -p prod --breakdown-costs -n finops-dt -y json -y pdf -d ./reports
aws-finops -C config.yaml --report-name override --trend
```

---

## Relatórios e Exportação

* Tipos suportados: `csv`, `json`, `pdf`
* Dashboard:

    * Colunas: Conta, períodos, custos por serviço, Budget, EC2
* Auditoria:

    * Colunas: Conta, Budget, NAT Gateway, EBS, EC2, LBs, Tags
* Exemplo:

```bash
aws-finops -p prod -n report-name -y csv -y json -y pdf -d ./out
```

---

## Fluxo interno e Arquitetura

**Arquitetura Hexagonal (Ports & Adapters):**

* Domain: entidades e interfaces
* Application: casos de uso
* Adapters:

    * driven: AWS SDK, exportação, config
    * driving: CLI (cobra)

**Principais componentes:**

* `cmd/aws-finops/main.go`
* `internal/adapter/driving/cli`
* `internal/application/usecase`
* `internal/adapter/driven/aws`
* `pkg/console`
* `pkg/version`

Fluxo:

1. Inicializa CLI e casos de uso
2. Lê config/flags
3. Executa dashboard/auditoria/tendência
4. Exibe no terminal e exporta relatórios
5. Verifica atualização (ignorada em `-dev`)

---

## Permissões AWS necessárias

**Dashboard e tendência:**

```
ce:GetCostAndUsage
budgets:DescribeBudgets
ec2:DescribeInstances
ec2:DescribeRegions
sts:GetCallerIdentity
```

**Auditoria:**

```
ec2:DescribeVolumes
ec2:DescribeAddresses
rds:DescribeDBInstances
lambda:ListFunctions
elasticloadbalancing:DescribeLoadBalancers
elasticloadbalancing:DescribeTargetGroups
elasticloadbalancing:DescribeTargetHealth
```

---

## Solução de problemas (Troubleshooting)

* `No AWS profiles found`: configure com `aws configure --profile <nome>`
* `credential validation failed`: renove STS/SSO
* Cost Explorer vazio: habilite-o na conta
* `AccessDenied`: ajuste políticas IAM
* PDF cortado: use JSON/CSV
* Versão incorreta: use `-ldflags` no build

---

## Observações de desempenho

* Processamento concorrente com worker pool
* Feedback visual com `pterm.MultiPrinter`
* Cache de clientes AWS
* `--combine` reduz chamadas redundantes

---

## Segurança

* Sem logging de credenciais
* Usa perfis padrão da AWS CLI
* Princípio de menor privilégio

---

## Screenshots

**Dashboard**

![Dashboard](./img/aws-finops-dashboard-go-v1.png)

**Tendência**

![Trend](./img/aws-finops-dashboard-go-trend.png)

**Auditoria**

![Audit](./img/aws-finops-dashboard-go-audit-report.png)

---

## Licença e Créditos

Inspirado em:

* [ravikiranvm/aws-finops-dashboard](https://github.com/ravikiranvm/aws-finops-dashboard) (Python)

Licença: **MIT**

Port Go e melhorias:

* [diillson/aws-finops-dashboard-go](https://github.com/diillson/aws-finops-dashboard-go)

Contribuições são bem-vindas — abra issues ou PRs com sugestões 🚀

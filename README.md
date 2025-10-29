# AWS FinOps Dashboard (Go) — CLI

Uma CLI para visualizar e auditar custos na AWS (FinOps), com suporte a múltiplos perfis, combinação por conta, exportação de relatórios (CSV/JSON/PDF), análise de tendências e auditoria de otimizações (NAT Gateways, LBs ociosos, recursos sem uso, S3 Lifecycle, SP/RI Coverage e mais).

---

## Sumário
- [Recursos (Features)](#recursos-features)
- [Pré-requisitos](#pré-requisitos)
- [Instalação e Build](#instalação-e-build)
  - [Build com Makefile (Recomendado)](#build-com-makefile-recomendado)
  - [Build Manual (ldflags)](#build-manual-ldflags)
- [Uso Rápido](#uso-rápido)
- [Flags da CLI](#flags-da-cli)
- [Arquivo de Configuração (TOML/YAML/JSON)](#arquivo-de-configuração-tomlyamljson)
- [Casos de Uso (Exemplos Práticos)](#casos-de-uso-exemplos-práticos)
- [Relatórios e Exportação](#relatórios-e-exportação)
- [Fluxo Interno e Arquitetura](#fluxo-interno-e-arquitetura)
- [Permissões AWS Necessárias](#permissões-aws-necessárias)
- [Solução de Problemas (Troubleshooting)](#solução-de-problemas-troubleshooting)
- [Observações de Desempenho](#observações-de-desempenho)
- [Segurança](#segurança)
- [Screenshots](#screenshots)
- [Licença e Créditos](#licença-e-créditos)

---

## Recursos (Features)

- **Dashboard de Custos**:
  - Custo do período anterior vs. atual e variação percentual.
  - Custos por serviço, com detalhamento opcional (`--breakdown-costs`).
  - Sumário de instâncias EC2 por estado.
  - Status de Budgets (limite, atual, forecast).
- **Análise de Tendências** (`--trend`): Gráfico de custos dos últimos 6 meses.
- **Auditoria Abrangente** (`--full-audit`):
  - **Auditoria Principal** (`--audit`):
    - NAT Gateways com alto custo.
    - Load Balancers ociosos.
    - Volumes EBS e EIPs sem uso.
    - EC2 paradas.
    - Recursos sem tags (EC2, RDS, Lambda).
    - VPC Endpoints (Interface) sem uso.
  - **Auditoria de Data Transfer** (`--transfer`):
    - Detalhamento de custos por categoria (Internet, Inter-Region, Cross-AZ, NAT).
    - Identificação dos principais serviços e tipos de uso que geram custos.
  - **Auditoria de CloudWatch Logs** (`--logs-audit`):
    - Identificação de Log Groups sem política de retenção (`Never Expire`).
    - Ordenação por tamanho para priorizar ações.
  - **Auditoria de S3** (`--s3-audit`):
    - Buckets sem política de Lifecycle.
    - Buckets com versionamento ativo sem regra para versões antigas.
    - Checagem de criptografia padrão.
    - Análise de configuração de acesso público (Public Access Block e heurísticas).
  - **Auditoria de Compromissos** (`--commitments`):
    - Análise de cobertura e utilização de Savings Plans (SP).
    - Análise de cobertura e utilização de Reserved Instances (RI).
- **Exportação Flexível**: CSV, JSON e PDF para todos os relatórios.
- **Configuração Simplificada**: Suporte a arquivos de configuração TOML, YAML ou JSON.
- **Interface Rica no Terminal**: Banner, barras de progresso paralelas (`pterm`), tabelas e gráficos.

---

## Pré-requisitos

- Go **1.24+** (recomendado).
- AWS CLI configurado com credenciais válidas.
- **Cost Explorer** habilitado na conta AWS.
- Permissões de IAM adequadas ([ver abaixo](#permissões-aws-necessárias)).

---

## Instalação e Build

Clone o repositório:
```bash
git clone https://github.com/diillson/aws-finops-dashboard-go.git
cd aws-finops-dashboard-go
````

### Build com Makefile (Recomendado)

O projeto inclui um `Makefile` que automatiza a injeção de informações de versão a partir do Git.

**Build de Produção:**

```bash
make build
```

O binário será gerado em `./bin/aws-finops`.

**Build de Desenvolvimento (rápido, sem ldflags):**

```bash
make build-dev
```

### Build Manual (ldflags)

Se preferir, você pode compilar manualmente. Use `-ldflags` para embutir a versão correta.

**Linux/macOS:**

```bash
VERSION=$(git describe --tags --abbrev=0)
COMMIT=$(git rev-parse --short HEAD)
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

go build -ldflags "-s -w \
  -X github.com/diillson/aws-finops-dashboard-go/pkg/version.Version=${VERSION} \
  -X github.com/diillson/aws-finops-dashboard-go/pkg/version.Commit=${COMMIT} \
  -X github.com/diillson/aws-finops-dashboard-go/pkg/version.BuildTime=${BUILD_TIME}" \
  -o bin/aws-finops ./cmd/aws-finops
```

---

## Uso Rápido

```bash
# Ajuda e versão
./bin/aws-finops --help
./bin/aws-finops --version

# Dashboard de custos para um perfil específico
./bin/aws-finops -p meu-perfil-prod

# Auditoria completa para todas as contas, com exportação
./bin/aws-finops --all --combine --full-audit -n full-audit-$(date +%Y%m%d) -y pdf -y json -d ./reports

# Análise de tendência de custos dos últimos 6 meses
./bin/aws-finops -p meu-perfil-prod --trend

# Auditoria específica de S3
./bin/aws-finops -p meu-perfil-prod --s3-audit
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
--audit                    Auditoria principal (recursos ociosos/sem tag)
--transfer                 Auditoria de custos de Data Transfer
--logs-audit               Auditoria de retenção de CloudWatch Logs
--s3-audit                 Auditoria de S3 (Lifecycle, Segurança)
--commitments              Auditoria de Savings Plans e RIs
--full-audit               Executa todas as auditorias em sequência
--breakdown-costs          Detalhamento de custos (usage-type)
--version                  Mostra a versão
--help                     Ajuda
```

Flags de linha de comando sobrescrevem as configurações do arquivo de configuração.

---

## Arquivo de Configuração (TOML/YAML/JSON)

Você pode centralizar suas configurações em um arquivo para facilitar o uso.
Exemplo de `config.toml`:

```toml
profiles = ["production", "development"]
regions = ["us-east-1", "us-west-2"]
combine = true
report_name = "aws-finops-monthly"
report_type = ["pdf", "json"]
dir = "/home/user/reports/aws"
time_range = 30
tag = ["Environment=Production"]
```

YAML
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

JSON
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

Uso:

```bash
./bin/aws-finops --config-file config.toml --full-audit
```

---

## Casos de Uso (Exemplos Práticos)

Gerar um relatório de auditoria completo e abrangente para todas as contas:

```bash
./bin/aws-finops --all --combine --full-audit \
  -n full-audit-report-$(date +%Y%m%d) \
  -y pdf -y json \
  -d ./reports/audits
```

Analisar a cobertura de Savings Plans e RIs nos últimos 60 dias:

```bash
./bin/aws-finops -p payer-account --commitments -t 60
```

Investigar custos de transferência de dados para a equipe de "Payments":

```bash
./bin/aws-finops -p prod --transfer -g Team=Payments
```

Verificar a higiene dos buckets S3 em todas as contas:

```bash
./bin/aws-finops --all --s3-audit
```

---

## Relatórios e Exportação

* **Formatos Suportados:** `csv`, `json`, `pdf`
* **Relatório de Auditoria Completa (`--full-audit`):**

    * **JSON:** Um único arquivo com a estrutura aninhada de todos os relatórios.
    * **PDF:** Um único documento com uma página de rosto e “capítulos” para cada auditoria.
    * **CSV:** Um pacote de arquivos (`..._main.csv`, `..._transfer.csv`, etc.), um para cada tipo de auditoria.

---

## Fluxo Interno e Arquitetura

O projeto segue a **Arquitetura Hexagonal (Ports & Adapters)**:

* **Domain:** Entidades de negócio e interfaces (ports).
* **Application:** Casos de uso que orquestram a lógica.
* **Adapters:**

    * Driven (Saída): AWS SDK, exportação de arquivos, leitura de configuração.
    * Driving (Entrada): CLI (Cobra).

---

## Permissões AWS Necessárias

Para que a ferramenta funcione com todos os recursos, a role ou usuário IAM precisa das seguintes permissões de leitura:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "BasePermissions",
      "Effect": "Allow",
      "Action": [
        "sts:GetCallerIdentity",
        "ec2:DescribeRegions"
      ],
      "Resource": "*"
    },
    {
      "Sid": "CostExplorerAndBudgets",
      "Effect": "Allow",
      "Action": [
        "ce:GetCostAndUsage",
        "ce:GetReservationCoverage",
        "ce:GetReservationUtilization",
        "ce:GetSavingsPlansCoverage",
        "ce:GetSavingsPlansUtilization",

        "budgets:ViewBudget",
        "budgets:DescribeBudgetAction",
        "budgets:DescribeBudgetActionsForBudget",
        "budgets:DescribeBudgetActionsForAccount",
        "budgets:DescribeBudgetActionHistories"
      ],
      "Resource": "*"
    },
    {
      "Sid": "ResourceInventoryAndAudit",
      "Effect": "Allow",
      "Action": [
        "ec2:DescribeInstances",
        "ec2:DescribeVolumes",
        "ec2:DescribeAddresses",
        "ec2:DescribeVpcEndpoints",
        "elasticloadbalancing:DescribeLoadBalancers",
        "elasticloadbalancing:DescribeTargetGroups",
        "elasticloadbalancing:DescribeTargetHealth",
        "rds:DescribeDBInstances",
        "lambda:ListFunctions",
        "lambda:ListTags"
      ],
      "Resource": "*"
    },
    {
      "Sid": "S3Audit",
      "Effect": "Allow",
      "Action": [
        "s3:ListAllMyBuckets",
        "s3:GetBucketLocation",
        "s3:GetBucketVersioning",

        "s3:GetLifecycleConfiguration",
        "s3:GetIntelligentTieringConfiguration",
        "s3:GetEncryptionConfiguration",

        "s3:GetBucketAcl",
        "s3:GetBucketPolicy",
        "s3:GetBucketPublicAccessBlock",
        "s3:GetAccountPublicAccessBlock"
      ],
      "Resource": "*"
    },
    {
      "Sid": "CloudWatchLogsAudit",
      "Effect": "Allow",
      "Action": [
        "logs:DescribeLogGroups"
      ],
      "Resource": "*"
    }
  ]
}
```

---

## Solução de Problemas (Troubleshooting)

* `No AWS profiles found`: Configure a AWS CLI com `aws configure --profile <nome>`.
* `credential validation failed`: Renove suas credenciais (ex: `aws sso login`).
* `DataUnavailableException`: Comum em contas-membro para relatórios de SP/RI. Execute na conta *payer* para obter dados completos.
* `AccessDenied`: Verifique se a política IAM possui todas as permissões listadas acima.
* Versão incorreta: Use `make build` para garantir que a versão do Git seja embutida corretamente.

---

## Observações de Desempenho

* **Processamento Concorrente:** Utiliza um pool de workers para auditar múltiplos perfis e regiões em paralelo.
* **Feedback Visual:** Barras de progresso paralelas (`pterm.MultiPrinter`) fornecem feedback claro sem poluir o terminal.
* **Cache de Clientes AWS:** Clientes do SDK são cacheados para reutilização, reduzindo a sobrecarga de inicialização.
* **`--combine`:** Reduz chamadas de API redundantes para perfis que compartilham a mesma conta AWS.

---

## Segurança

* A ferramenta **não armazena nem faz log de credenciais**.
* Utiliza os perfis e mecanismos de autenticação padrão da AWS CLI.
* A política IAM recomendada segue o **princípio de menor privilégio (somente leitura)**.

---

## Screenshots

**Dashboard**
![Dashboard](/img/aws-finops-dashboard-go-v1.png)

**Tendência**
![Trend](/img/aws-finops-dashboard-go-trend.png)

**Auditoria**
![Audit](/img/aws-finops-dashboard-go-audit-report.png)

---

## Licença e Créditos

Inspirado em:

* [ravikiranvm/aws-finops-dashboard](https://github.com/ravikiranvm/aws-finops-dashboard) (Python)

**Licença:** MIT

Port para Go e melhorias:

* [diillson/aws-finops-dashboard-go](https://github.com/diillson/aws-finops-dashboard-go)

Contribuições são bem-vindas — abra issues ou PRs com sugestões 🚀

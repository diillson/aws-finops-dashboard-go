package aws

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/budgets"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	ceTypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/diillson/aws-finops-dashboard-go/internal/domain/entity"
	"github.com/diillson/aws-finops-dashboard-go/internal/domain/repository"
)

// AWSRepositoryImpl implementa o AWSRepository com cache de clientes.
type AWSRepositoryImpl struct {
	cfgCache    map[string]aws.Config
	clientCache map[string]interface{}
	mu          sync.Mutex
}

// NewAWSRepository cria uma nova implementação do AWSRepository.
func NewAWSRepository() repository.AWSRepository {
	return &AWSRepositoryImpl{
		cfgCache:    make(map[string]aws.Config),
		clientCache: make(map[string]interface{}),
	}
}

// GetSession é um método placeholder para compatibilidade com a interface,
// já que o SDK v2 gerencia sessões implicitamente através da config.
func (r *AWSRepositoryImpl) GetSession(ctx context.Context, profile string) (string, error) {
	_, err := r.getAWSConfig(ctx, profile)
	if err != nil {
		return "", err
	}
	return profile, nil
}

// GetAllRegions é um alias para GetAccessibleRegions para manter a compatibilidade.
func (r *AWSRepositoryImpl) GetAllRegions(ctx context.Context, profile string) ([]string, error) {
	return r.GetAccessibleRegions(ctx, profile)
}

func (r *AWSRepositoryImpl) getAWSConfig(ctx context.Context, profile string) (aws.Config, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if cfg, ok := r.cfgCache[profile]; ok {
		return cfg, nil
	}

	cfg, err := config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(profile))
	if err != nil {
		return aws.Config{}, fmt.Errorf("failed to load AWS config for profile %s: %w", profile, err)
	}

	r.cfgCache[profile] = cfg
	return cfg, nil
}

func (r *AWSRepositoryImpl) getServiceClient(ctx context.Context, profile, region, service string) (interface{}, error) {
	cacheKey := fmt.Sprintf("%s-%s-%s", profile, region, service)

	r.mu.Lock()
	if client, ok := r.clientCache[cacheKey]; ok {
		r.mu.Unlock()
		return client, nil
	}
	r.mu.Unlock()

	cfg, err := r.getAWSConfig(ctx, profile)
	if err != nil {
		return nil, err
	}

	regionalCfg := cfg.Copy()
	if region != "" {
		regionalCfg.Region = region
	}

	var client interface{}
	switch service {
	case "sts":
		client = sts.NewFromConfig(regionalCfg)
	case "ec2":
		client = ec2.NewFromConfig(regionalCfg)
	case "costexplorer":
		regionalCfg.Region = "us-east-1"
		client = costexplorer.NewFromConfig(regionalCfg)
	case "budgets":
		regionalCfg.Region = "us-east-1"
		client = budgets.NewFromConfig(regionalCfg)
	case "rds":
		client = rds.NewFromConfig(regionalCfg)
	case "lambda":
		client = lambda.NewFromConfig(regionalCfg)
	case "elbv2":
		client = elasticloadbalancingv2.NewFromConfig(regionalCfg)
	default:
		return nil, fmt.Errorf("unsupported service: %s", service)
	}

	r.mu.Lock()
	r.clientCache[cacheKey] = client
	r.mu.Unlock()

	return client, nil
}

func (r *AWSRepositoryImpl) GetAWSProfiles() []string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return []string{"default"}
	}

	credentialsPath := filepath.Join(homeDir, ".aws", "credentials")
	configPath := filepath.Join(homeDir, ".aws", "config")

	profiles := make(map[string]bool)
	profileRegex := regexp.MustCompile(`\[([^]]+)\]`)

	parseFile := func(path string, isConfig bool) {
		content, err := os.ReadFile(path)
		if err != nil {
			return
		}
		matches := profileRegex.FindAllStringSubmatch(string(content), -1)
		for _, match := range matches {
			profileName := match[1]
			if isConfig {
				profileName = strings.TrimPrefix(profileName, "profile ")
			}
			profiles[profileName] = true
		}
	}

	parseFile(credentialsPath, false)
	parseFile(configPath, true)

	if len(profiles) == 0 {
		profiles["default"] = true
	}

	result := make([]string, 0, len(profiles))
	for profile := range profiles {
		result = append(result, profile)
	}
	sort.Strings(result)
	return result
}

func (r *AWSRepositoryImpl) GetAccountID(ctx context.Context, profile string) (string, error) {
	client, err := r.getServiceClient(ctx, profile, "us-east-1", "sts")
	if err != nil {
		return "", err
	}
	stsClient := client.(*sts.Client)

	result, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("error getting account ID for profile %s: %w", profile, err)
	}
	return *result.Account, nil
}

func (r *AWSRepositoryImpl) GetAccessibleRegions(ctx context.Context, profile string) ([]string, error) {
	defaultRegions := []string{"us-east-1", "us-east-2", "us-west-1", "us-west-2", "eu-west-1", "eu-central-1"}

	client, err := r.getServiceClient(ctx, profile, "us-east-1", "ec2")
	if err != nil {
		return defaultRegions, fmt.Errorf("could not create EC2 client to list regions: %w", err)
	}
	ec2Client := client.(*ec2.Client)

	regionsOutput, err := ec2Client.DescribeRegions(ctx, &ec2.DescribeRegionsInput{AllRegions: aws.Bool(false)})
	if err != nil {
		return defaultRegions, nil
	}

	accessibleRegions := make([]string, 0, len(regionsOutput.Regions))
	for _, region := range regionsOutput.Regions {
		accessibleRegions = append(accessibleRegions, *region.RegionName)
	}
	return accessibleRegions, nil
}

func (r *AWSRepositoryImpl) GetEC2Summary(ctx context.Context, profile string, regions []string) (entity.EC2Summary, error) {
	summary := make(entity.EC2Summary)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, region := range regions {
		wg.Add(1)
		go func(rgn string) {
			defer wg.Done()
			client, err := r.getServiceClient(ctx, profile, rgn, "ec2")
			if err != nil {
				return
			}
			ec2Client := client.(*ec2.Client)

			paginator := ec2.NewDescribeInstancesPaginator(ec2Client, &ec2.DescribeInstancesInput{})
			for paginator.HasMorePages() {
				output, err := paginator.NextPage(ctx)
				if err != nil {
					continue
				}
				mu.Lock()
				for _, reservation := range output.Reservations {
					for _, instance := range reservation.Instances {
						summary[string(instance.State.Name)]++
					}
				}
				mu.Unlock()
			}
		}(region)
	}
	wg.Wait()

	mu.Lock()
	if _, ok := summary["running"]; !ok {
		summary["running"] = 0
	}
	if _, ok := summary["stopped"]; !ok {
		summary["stopped"] = 0
	}
	mu.Unlock()

	return summary, nil
}

func (r *AWSRepositoryImpl) GetCostData(ctx context.Context, profile string, timeRange *int, tags []string, breakdown bool) (entity.CostData, error) {
	client, err := r.getServiceClient(ctx, profile, "", "costexplorer")
	if err != nil {
		return entity.CostData{}, err
	}
	ceClient := client.(*costexplorer.Client)

	today := time.Now().UTC()
	var startDate, endDate, prevStartDate, prevEndDate time.Time
	currentPeriodName, previousPeriodName := "Current month's cost", "Last month's cost"

	if timeRange != nil && *timeRange > 0 {
		endDate = today
		startDate = today.AddDate(0, 0, -(*timeRange))
		prevEndDate = startDate.AddDate(0, 0, -1)
		prevStartDate = prevEndDate.AddDate(0, 0, -(*timeRange))
		currentPeriodName = fmt.Sprintf("Current %d days cost", *timeRange)
		previousPeriodName = fmt.Sprintf("Previous %d days cost", *timeRange)
	} else {
		startDate = time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
		endDate = today
		if startDate.Day() == endDate.Day() && startDate.Month() == endDate.Month() && startDate.Year() == endDate.Year() {
			endDate = endDate.AddDate(0, 0, 1)
		}
		prevEndDate = startDate.AddDate(0, 0, -1)
		prevStartDate = time.Date(prevEndDate.Year(), prevEndDate.Month(), 1, 0, 0, 0, 0, time.UTC)
	}

	filter, err := parseTagFilter(tags)
	if err != nil {
		return entity.CostData{}, err
	}

	var costData entity.CostData
	var wg sync.WaitGroup
	errChan := make(chan error, 3)

	wg.Add(1)
	go func() {
		defer wg.Done()
		cost, err := r.getCostForPeriod(ctx, ceClient, startDate, endDate, filter)
		if err != nil {
			errChan <- fmt.Errorf("failed to get current period cost: %w", err)
			return
		}
		costData.CurrentMonthCost = cost
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		cost, err := r.getCostForPeriod(ctx, ceClient, prevStartDate, prevEndDate, filter)
		if err != nil {
			errChan <- fmt.Errorf("failed to get previous period cost: %w", err)
			return
		}
		costData.LastMonthCost = cost
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		// Passa a flag 'breakdown' para a função de busca
		services, err := r.getCostByService(ctx, ceClient, startDate, endDate, filter, breakdown)
		if err != nil {
			errChan <- fmt.Errorf("failed to get cost by service: %w", err)
			return
		}
		costData.CurrentMonthCostByService = services
	}()

	wg.Wait()
	close(errChan)

	if len(errChan) > 0 {
		return entity.CostData{}, <-errChan
	}

	costData.AccountID, _ = r.GetAccountID(ctx, profile)
	costData.Budgets, _ = r.GetBudgets(ctx, profile)
	costData.CurrentPeriodName, costData.PreviousPeriodName = currentPeriodName, previousPeriodName
	costData.CurrentPeriodStart, costData.CurrentPeriodEnd = startDate, endDate
	costData.PreviousPeriodStart, costData.PreviousPeriodEnd = prevStartDate, prevEndDate
	if timeRange != nil {
		costData.TimeRange = *timeRange
	}

	return costData, nil
}

func (r *AWSRepositoryImpl) getCostForPeriod(ctx context.Context, client *costexplorer.Client, start, end time.Time, filter *ceTypes.Expression) (float64, error) {
	input := &costexplorer.GetCostAndUsageInput{
		TimePeriod: &ceTypes.DateInterval{
			Start: aws.String(start.Format("2006-01-02")),
			End:   aws.String(end.Format("2006-01-02")),
		},
		Granularity: ceTypes.GranularityMonthly,
		Metrics:     []string{"UnblendedCost"},
		Filter:      filter,
	}

	result, err := client.GetCostAndUsage(ctx, input)
	if err != nil {
		return 0, err
	}

	var totalCost float64
	if len(result.ResultsByTime) > 0 && result.ResultsByTime[0].Total != nil {
		if val, ok := result.ResultsByTime[0].Total["UnblendedCost"]; ok {
			cost, _ := strconv.ParseFloat(*val.Amount, 64)
			totalCost = cost
		}
	}
	return totalCost, nil
}

func (r *AWSRepositoryImpl) getCostByService(ctx context.Context, client *costexplorer.Client, start, end time.Time, filter *ceTypes.Expression, breakdown bool) ([]entity.ServiceCost, error) {
	input := &costexplorer.GetCostAndUsageInput{
		TimePeriod: &ceTypes.DateInterval{
			Start: aws.String(start.Format("2006-01-02")),
			End:   aws.String(end.Format("2006-01-02")),
		},
		Granularity: ceTypes.GranularityMonthly,
		Metrics:     []string{"UnblendedCost"},
		GroupBy: []ceTypes.GroupDefinition{
			{Type: ceTypes.GroupDefinitionTypeDimension, Key: aws.String("SERVICE")},
		},
		Filter: filter,
	}

	result, err := client.GetCostAndUsage(ctx, input)
	if err != nil {
		return nil, err
	}

	var serviceCosts []entity.ServiceCost
	if len(result.ResultsByTime) > 0 {
		for _, group := range result.ResultsByTime[0].Groups {
			cost, _ := strconv.ParseFloat(*group.Metrics["UnblendedCost"].Amount, 64)
			if cost > 0.001 {
				sc := entity.ServiceCost{
					ServiceName: group.Keys[0],
					Cost:        cost,
				}

				servicesToBreakdown := map[string]bool{
					"EC2-Other":                    true,
					"Amazon API Gateway":           true,
					"Amazon Virtual Private Cloud": true,
				}

				if breakdown && servicesToBreakdown[sc.ServiceName] {
					breakdownCosts, err := r.getCostBreakdownForService(ctx, client, start, end, filter, sc.ServiceName)
					if err == nil {
						sc.SubCosts = breakdownCosts
					}
				}

				serviceCosts = append(serviceCosts, sc)
			}
		}
	}

	sort.Slice(serviceCosts, func(i, j int) bool {
		return serviceCosts[i].Cost > serviceCosts[j].Cost
	})

	return serviceCosts, nil
}

func (r *AWSRepositoryImpl) GetUnusedVpcEndpoints(ctx context.Context, profile string, regions []string) (entity.UnusedVpcEndpoints, error) {
	unusedEndpoints := make(entity.UnusedVpcEndpoints)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, region := range regions {
		wg.Add(1)
		go func(rgn string) {
			defer wg.Done()
			client, err := r.getServiceClient(ctx, profile, rgn, "ec2")
			if err != nil {
				return
			}
			ec2Client := client.(*ec2.Client)

			// Filtra por endpoints do tipo "Interface" que estão disponíveis
			endpoints, err := ec2Client.DescribeVpcEndpoints(ctx, &ec2.DescribeVpcEndpointsInput{
				Filters: []ec2Types.Filter{
					{Name: aws.String("vpc-endpoint-type"), Values: []string{"Interface"}},
					{Name: aws.String("vpc-endpoint-state"), Values: []string{"available"}},
				},
			})
			if err != nil {
				return
			}

			var regionUnusedEndpoints []string
			for _, ep := range endpoints.VpcEndpoints {
				// Um Interface Endpoint funcional deve ter pelo menos uma Network Interface.
				// Se a lista de IDs de Network Interface estiver vazia, o endpoint não está servindo tráfego.
				if len(ep.NetworkInterfaceIds) == 0 {
					regionUnusedEndpoints = append(regionUnusedEndpoints, *ep.VpcEndpointId)
				}
			}

			if len(regionUnusedEndpoints) > 0 {
				mu.Lock()
				unusedEndpoints[rgn] = regionUnusedEndpoints
				mu.Unlock()
			}
		}(region)
	}

	wg.Wait()
	return unusedEndpoints, nil
}

func (r *AWSRepositoryImpl) getCostBreakdownForService(ctx context.Context, client *costexplorer.Client, start, end time.Time, filter *ceTypes.Expression, serviceName string) ([]entity.ServiceCost, error) {
	serviceFilter := &ceTypes.Expression{
		Dimensions: &ceTypes.DimensionValues{
			Key:    "SERVICE",
			Values: []string{serviceName},
		},
	}

	finalFilter := serviceFilter
	if filter != nil {
		finalFilter = &ceTypes.Expression{
			And: []ceTypes.Expression{*filter, *serviceFilter},
		}
	}

	input := &costexplorer.GetCostAndUsageInput{
		TimePeriod: &ceTypes.DateInterval{
			Start: aws.String(start.Format("2006-01-02")),
			End:   aws.String(end.Format("2006-01-02")),
		},
		Granularity: ceTypes.GranularityMonthly,
		Metrics:     []string{"UnblendedCost"},
		GroupBy: []ceTypes.GroupDefinition{
			{Type: ceTypes.GroupDefinitionTypeDimension, Key: aws.String("USAGE_TYPE")},
		},
		Filter: finalFilter,
	}

	result, err := client.GetCostAndUsage(ctx, input)
	if err != nil {
		return nil, err
	}

	var breakdownCosts []entity.ServiceCost
	if len(result.ResultsByTime) > 0 {
		for _, group := range result.ResultsByTime[0].Groups {
			cost, _ := strconv.ParseFloat(*group.Metrics["UnblendedCost"].Amount, 64)
			if cost > 0.001 {
				usageType := group.Keys[0]
				parts := strings.Split(usageType, "-")
				if len(parts) > 1 {
					// Remove o prefixo da região (ex: USE2-DataTransfer-Out-Bytes -> DataTransfer-Out-Bytes)
					if len(parts[0]) == 4 && (strings.HasPrefix(parts[0], "U") || strings.HasPrefix(parts[0], "E") || strings.HasPrefix(parts[0], "AP")) {
						usageType = strings.Join(parts[1:], "-")
					}
				}

				breakdownCosts = append(breakdownCosts, entity.ServiceCost{
					ServiceName: usageType,
					Cost:        cost,
				})
			}
		}
	}

	sort.Slice(breakdownCosts, func(i, j int) bool {
		return breakdownCosts[i].Cost > breakdownCosts[j].Cost
	})

	return breakdownCosts, nil
}

func (r *AWSRepositoryImpl) GetBudgets(ctx context.Context, profile string) ([]entity.BudgetInfo, error) {
	client, err := r.getServiceClient(ctx, profile, "", "budgets")
	if err != nil {
		return nil, err
	}
	budgetsClient := client.(*budgets.Client)

	accountID, err := r.GetAccountID(ctx, profile)
	if err != nil {
		return nil, err
	}

	result, err := budgetsClient.DescribeBudgets(ctx, &budgets.DescribeBudgetsInput{
		AccountId: aws.String(accountID),
	})
	if err != nil {
		return nil, nil // Not a fatal error
	}

	budgetsData := []entity.BudgetInfo{}
	for _, budget := range result.Budgets {
		b := entity.BudgetInfo{Name: *budget.BudgetName}
		if budget.BudgetLimit != nil {
			b.Limit, _ = strconv.ParseFloat(*budget.BudgetLimit.Amount, 64)
		}
		if budget.CalculatedSpend != nil && budget.CalculatedSpend.ActualSpend != nil {
			b.Actual, _ = strconv.ParseFloat(*budget.CalculatedSpend.ActualSpend.Amount, 64)
		}
		if budget.CalculatedSpend.ForecastedSpend != nil {
			b.Forecast, _ = strconv.ParseFloat(*budget.CalculatedSpend.ForecastedSpend.Amount, 64)
		}
		budgetsData = append(budgetsData, b)
	}

	return budgetsData, nil
}

func parseTagFilter(tags []string) (*ceTypes.Expression, error) {
	if len(tags) == 0 {
		return nil, nil
	}

	var expressions []ceTypes.Expression
	for _, t := range tags {
		parts := strings.SplitN(t, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tag format: %s", t)
		}
		expressions = append(expressions, ceTypes.Expression{
			Tags: &ceTypes.TagValues{
				Key:    aws.String(parts[0]),
				Values: []string{parts[1]},
			},
		})
	}

	if len(expressions) == 1 {
		return &expressions[0], nil
	}

	return &ceTypes.Expression{And: expressions}, nil
}

func (r *AWSRepositoryImpl) GetTrendData(ctx context.Context, profile string, tags []string) (map[string]interface{}, error) {
	client, err := r.getServiceClient(ctx, profile, "", "costexplorer")
	if err != nil {
		return nil, err
	}
	ceClient := client.(*costexplorer.Client)

	accountID, _ := r.GetAccountID(ctx, profile)

	today := time.Now().UTC()
	endDate := today
	startDate := today.AddDate(0, -6, 0)
	startDate = time.Date(startDate.Year(), startDate.Month(), 1, 0, 0, 0, 0, time.UTC)

	filter, err := parseTagFilter(tags)
	if err != nil {
		return nil, err
	}

	input := &costexplorer.GetCostAndUsageInput{
		TimePeriod: &ceTypes.DateInterval{
			Start: aws.String(startDate.Format("2006-01-02")),
			End:   aws.String(endDate.Format("2006-01-02")),
		},
		Granularity: ceTypes.GranularityMonthly,
		Metrics:     []string{"UnblendedCost"},
		Filter:      filter,
	}

	result, err := ceClient.GetCostAndUsage(ctx, input)
	if err != nil {
		return nil, err
	}

	monthlyCosts := []entity.MonthlyCost{}
	for _, period := range result.ResultsByTime {
		month, _ := time.Parse("2006-01-02", *period.TimePeriod.Start)
		cost, _ := strconv.ParseFloat(*period.Total["UnblendedCost"].Amount, 64)
		monthlyCosts = append(monthlyCosts, entity.MonthlyCost{
			Month: month.Format("Jan 2006"),
			Cost:  cost,
		})
	}

	return map[string]interface{}{
		"monthly_costs": monthlyCosts,
		"account_id":    accountID,
	}, nil
}

func (r *AWSRepositoryImpl) GetStoppedInstances(ctx context.Context, profile string, regions []string) (entity.StoppedEC2Instances, error) {
	stopped := make(entity.StoppedEC2Instances)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, region := range regions {
		wg.Add(1)
		go func(rgn string) {
			defer wg.Done()
			client, err := r.getServiceClient(ctx, profile, rgn, "ec2")
			if err != nil {
				return
			}
			ec2Client := client.(*ec2.Client)

			result, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
				Filters: []ec2Types.Filter{{Name: aws.String("instance-state-name"), Values: []string{"stopped"}}},
			})
			if err != nil {
				return
			}

			var instanceIDs []string
			for _, res := range result.Reservations {
				for _, inst := range res.Instances {
					instanceIDs = append(instanceIDs, *inst.InstanceId)
				}
			}
			if len(instanceIDs) > 0 {
				mu.Lock()
				stopped[rgn] = instanceIDs
				mu.Unlock()
			}
		}(region)
	}
	wg.Wait()
	return stopped, nil
}

func (r *AWSRepositoryImpl) GetUnusedVolumes(ctx context.Context, profile string, regions []string) (entity.UnusedVolumes, error) {
	unused := make(entity.UnusedVolumes)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, region := range regions {
		wg.Add(1)
		go func(rgn string) {
			defer wg.Done()
			client, err := r.getServiceClient(ctx, profile, rgn, "ec2")
			if err != nil {
				return
			}
			ec2Client := client.(*ec2.Client)

			result, err := ec2Client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
				Filters: []ec2Types.Filter{{Name: aws.String("status"), Values: []string{"available"}}},
			})
			if err != nil {
				return
			}

			var volIDs []string
			for _, vol := range result.Volumes {
				volIDs = append(volIDs, *vol.VolumeId)
			}
			if len(volIDs) > 0 {
				mu.Lock()
				unused[rgn] = volIDs
				mu.Unlock()
			}
		}(region)
	}
	wg.Wait()
	return unused, nil
}

func (r *AWSRepositoryImpl) GetUnusedEIPs(ctx context.Context, profile string, regions []string) (entity.UnusedEIPs, error) {
	eips := make(entity.UnusedEIPs)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, region := range regions {
		wg.Add(1)
		go func(rgn string) {
			defer wg.Done()
			client, err := r.getServiceClient(ctx, profile, rgn, "ec2")
			if err != nil {
				return
			}
			ec2Client := client.(*ec2.Client)

			result, err := ec2Client.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{})
			if err != nil {
				return
			}

			var freeIPs []string
			for _, addr := range result.Addresses {
				if addr.AssociationId == nil {
					freeIPs = append(freeIPs, *addr.PublicIp)
				}
			}
			if len(freeIPs) > 0 {
				mu.Lock()
				eips[rgn] = freeIPs
				mu.Unlock()
			}
		}(region)
	}
	wg.Wait()
	return eips, nil
}

// GetNatGatewayCost retorna o custo de processamento de dados para cada NAT Gateway.
func (r *AWSRepositoryImpl) GetNatGatewayCost(ctx context.Context, profile string, timeRange *int, tags []string) ([]entity.NatGatewayCost, error) {
	client, err := r.getServiceClient(ctx, profile, "", "costexplorer")
	if err != nil {
		return nil, err
	}
	ceClient := client.(*costexplorer.Client)

	// Define o período de tempo (usa a mesma lógica do GetCostData)
	today := time.Now().UTC()
	var startDate, endDate time.Time
	if timeRange != nil && *timeRange > 0 {
		endDate = today
		startDate = today.AddDate(0, 0, -(*timeRange))
	} else {
		startDate = time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
		endDate = today
	}

	// Filtro para pegar apenas custos de processamento de NAT Gateway
	usageTypeFilter := &ceTypes.Expression{
		Dimensions: &ceTypes.DimensionValues{
			Key:          "USAGE_TYPE",
			Values:       []string{"NatGateway-Bytes"}, // O Usage Type pode variar um pouco, ex: EU-NatGateway-Bytes. Usamos um filtro de string.
			MatchOptions: []ceTypes.MatchOption{ceTypes.MatchOptionContains},
		},
	}

	finalFilter := usageTypeFilter
	tagFilter, err := parseTagFilter(tags)
	if err == nil && tagFilter != nil {
		finalFilter = &ceTypes.Expression{
			And: []ceTypes.Expression{*tagFilter, *usageTypeFilter},
		}
	}

	input := &costexplorer.GetCostAndUsageInput{
		TimePeriod: &ceTypes.DateInterval{
			Start: aws.String(startDate.Format("2006-01-02")),
			End:   aws.String(endDate.Format("2006-01-02")),
		},
		Granularity: ceTypes.GranularityMonthly,
		Metrics:     []string{"UnblendedCost"},
		Filter:      finalFilter,
		GroupBy: []ceTypes.GroupDefinition{
			{Type: ceTypes.GroupDefinitionTypeDimension, Key: aws.String("RESOURCE_ID")},
			{Type: ceTypes.GroupDefinitionTypeDimension, Key: aws.String("REGION")},
		},
	}

	result, err := ceClient.GetCostAndUsage(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get NAT Gateway costs: %w", err)
	}

	var natCosts []entity.NatGatewayCost
	if len(result.ResultsByTime) > 0 {
		for _, group := range result.ResultsByTime[0].Groups {
			cost, _ := strconv.ParseFloat(*group.Metrics["UnblendedCost"].Amount, 64)
			// Ignora NAT Gateways sem custo significativo
			if cost > 0.1 { // Limiar de $0.10 para ser relevante
				resourceID := group.Keys[0]
				region := group.Keys[1]

				natCosts = append(natCosts, entity.NatGatewayCost{
					ResourceID: resourceID,
					Cost:       cost,
					Region:     region,
				})
			}
		}
	}

	// Ordena do mais caro para o mais barato
	sort.Slice(natCosts, func(i, j int) bool {
		return natCosts[i].Cost > natCosts[j].Cost
	})

	return natCosts, nil
}

func (r *AWSRepositoryImpl) GetUntaggedResources(ctx context.Context, profile string, regions []string) (entity.UntaggedResources, error) {
	untagged := make(entity.UntaggedResources)
	var mu sync.Mutex

	initService := func(service string) {
		mu.Lock()
		if _, ok := untagged[service]; !ok {
			untagged[service] = make(map[string][]string)
		}
		mu.Unlock()
	}

	var wg sync.WaitGroup
	for _, region := range regions {
		wg.Add(1)
		go func(rgn string) {
			defer wg.Done()

			// EC2
			initService("EC2")
			ec2Client, err := r.getServiceClient(ctx, profile, rgn, "ec2")
			if err == nil {
				if insts, err := ec2Client.(*ec2.Client).DescribeInstances(ctx, &ec2.DescribeInstancesInput{}); err == nil {
					var untaggedEC2 []string
					for _, res := range insts.Reservations {
						for _, inst := range res.Instances {
							if len(inst.Tags) == 0 {
								untaggedEC2 = append(untaggedEC2, *inst.InstanceId)
							}
						}
					}
					if len(untaggedEC2) > 0 {
						mu.Lock()
						untagged["EC2"][rgn] = untaggedEC2
						mu.Unlock()
					}
				}
			}

			// RDS
			initService("RDS")
			rdsClient, err := r.getServiceClient(ctx, profile, rgn, "rds")
			if err == nil {
				if dbs, err := rdsClient.(*rds.Client).DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{}); err == nil {
					var untaggedRDS []string
					for _, db := range dbs.DBInstances {
						if len(db.TagList) == 0 {
							untaggedRDS = append(untaggedRDS, *db.DBInstanceIdentifier)
						}
					}
					if len(untaggedRDS) > 0 {
						mu.Lock()
						untagged["RDS"][rgn] = untaggedRDS
						mu.Unlock()
					}
				}
			}

			// Lambda
			initService("Lambda")
			lambdaClient, err := r.getServiceClient(ctx, profile, rgn, "lambda")
			if err == nil {
				if funcs, err := lambdaClient.(*lambda.Client).ListFunctions(ctx, &lambda.ListFunctionsInput{}); err == nil {
					var untaggedLambda []string
					for _, fn := range funcs.Functions {
						if tags, err := lambdaClient.(*lambda.Client).ListTags(ctx, &lambda.ListTagsInput{Resource: fn.FunctionArn}); err == nil && len(tags.Tags) == 0 {
							untaggedLambda = append(untaggedLambda, *fn.FunctionName)
						}
					}
					if len(untaggedLambda) > 0 {
						mu.Lock()
						untagged["Lambda"][rgn] = untaggedLambda
						mu.Unlock()
					}
				}
			}
		}(region)
	}
	wg.Wait()
	return untagged, nil
}

// GetIdleLoadBalancers retorna Load Balancers (v2) que não possuem targets saudáveis.
func (r *AWSRepositoryImpl) GetIdleLoadBalancers(ctx context.Context, profile string, regions []string) (entity.IdleLoadBalancers, error) {
	idleLBs := make(entity.IdleLoadBalancers)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, region := range regions {
		wg.Add(1)
		go func(rgn string) {
			defer wg.Done()
			client, err := r.getServiceClient(ctx, profile, rgn, "elbv2")
			if err != nil {
				return
			}
			elbv2Client := client.(*elasticloadbalancingv2.Client)

			// 1. Listar todos os Load Balancers na região
			lbsOutput, err := elbv2Client.DescribeLoadBalancers(ctx, &elasticloadbalancingv2.DescribeLoadBalancersInput{})
			if err != nil {
				return
			}

			var regionIdleLBs []string

			for _, lb := range lbsOutput.LoadBalancers {
				lbArn := *lb.LoadBalancerArn
				lbName := *lb.LoadBalancerName

				// 2. Encontrar os Target Groups associados a este LB
				tgOutput, err := elbv2Client.DescribeTargetGroups(ctx, &elasticloadbalancingv2.DescribeTargetGroupsInput{
					LoadBalancerArn: &lbArn,
				})
				if err != nil || len(tgOutput.TargetGroups) == 0 {
					// Se não tem target groups, é ocioso por definição.
					regionIdleLBs = append(regionIdleLBs, lbName)
					continue
				}

				isCompletelyIdle := true
				for _, tg := range tgOutput.TargetGroups {
					// 3. Verificar a saúde dos targets em cada Target Group
					healthOutput, err := elbv2Client.DescribeTargetHealth(ctx, &elasticloadbalancingv2.DescribeTargetHealthInput{
						TargetGroupArn: tg.TargetGroupArn,
					})
					if err != nil {
						continue
					}

					// Se encontrarmos qualquer target (independente do estado), já não é 100% ocioso.
					// Uma verificação mais estrita poderia ser `if len(healthOutput.TargetHealthDescriptions) > 0`
					// e depois verificar o estado. Para simplificar, consideramos que se há targets, não é ocioso.
					if len(healthOutput.TargetHealthDescriptions) > 0 {
						isCompletelyIdle = false
						break
					}
				}

				if isCompletelyIdle {
					regionIdleLBs = append(regionIdleLBs, lbName)
				}
			}

			if len(regionIdleLBs) > 0 {
				mu.Lock()
				idleLBs[rgn] = regionIdleLBs
				mu.Unlock()
			}
		}(region)
	}

	wg.Wait()
	return idleLBs, nil
}

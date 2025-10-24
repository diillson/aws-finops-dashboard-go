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
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	ceTypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
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
	case "s3":
		client = s3.NewFromConfig(regionalCfg)
	case "cloudwatchlogs":
		client = cloudwatchlogs.NewFromConfig(regionalCfg)
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

// GetDataTransferBreakdown retorna um relatório detalhado de custos de Data Transfer.
// Ele agrega por categorias (Internet, Inter-Region, Cross-AZ/Regional, NAT Gateway, Other)
// e também retorna as Top Lines por (Service, UsageType).
func (r *AWSRepositoryImpl) GetDataTransferBreakdown(ctx context.Context, profile string, timeRange *int, tags []string) (entity.DataTransferReport, error) {
	client, err := r.getServiceClient(ctx, profile, "", "costexplorer")
	if err != nil {
		return entity.DataTransferReport{}, err
	}
	ceClient := client.(*costexplorer.Client)

	// Define período (mesma lógica de GetCostData)
	today := time.Now().UTC()
	var startDate, endDate time.Time
	periodName := "Current month's data transfer"

	if timeRange != nil && *timeRange > 0 {
		endDate = today
		startDate = today.AddDate(0, 0, -(*timeRange))
		periodName = fmt.Sprintf("Current %d days data transfer", *timeRange)
	} else {
		startDate = time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
		endDate = today
		// ajuste para incluir hoje quando estamos no primeiro dia, mesma abordagem do GetCostData
		if startDate.Day() == endDate.Day() && startDate.Month() == endDate.Month() && startDate.Year() == endDate.Year() {
			endDate = endDate.AddDate(0, 0, 1)
		}
	}

	filter, err := parseTagFilter(tags)
	if err != nil {
		return entity.DataTransferReport{}, err
	}

	input := &costexplorer.GetCostAndUsageInput{
		TimePeriod: &ceTypes.DateInterval{
			Start: aws.String(startDate.Format("2006-01-02")),
			End:   aws.String(endDate.Format("2006-01-02")),
		},
		Granularity: ceTypes.GranularityMonthly,
		Metrics:     []string{"UnblendedCost"},
		GroupBy: []ceTypes.GroupDefinition{
			{Type: ceTypes.GroupDefinitionTypeDimension, Key: aws.String("SERVICE")},
			{Type: ceTypes.GroupDefinitionTypeDimension, Key: aws.String("USAGE_TYPE")},
		},
		Filter: filter,
	}

	result, err := ceClient.GetCostAndUsage(ctx, input)
	if err != nil {
		return entity.DataTransferReport{}, fmt.Errorf("failed to get data transfer breakdown: %w", err)
	}

	categoryTotals := map[string]float64{
		"Internet":          0,
		"Inter-Region":      0,
		"Cross-AZ/Regional": 0,
		"NAT Gateway":       0,
		"Other":             0,
	}
	var total float64
	var lines []entity.DataTransferLine

	if len(result.ResultsByTime) > 0 {
		for _, group := range result.ResultsByTime[0].Groups {
			if len(group.Keys) < 2 {
				continue
			}
			service := group.Keys[0]
			usage := group.Keys[1]
			amountStr := group.Metrics["UnblendedCost"].Amount
			if amountStr == nil {
				continue
			}
			cost, _ := strconv.ParseFloat(*amountStr, 64)
			if cost < 0.001 {
				continue
			}

			// Classifica a linha
			category, relevant := classifyUsageType(usage)
			if !relevant {
				// Ignora completamente itens irrelevantes ao tema "transfer"
				continue
			}

			categoryTotals[category] += cost
			total += cost
			lines = append(lines, entity.DataTransferLine{
				Service:   service,
				UsageType: usage,
				Cost:      cost,
			})
		}
	}

	// Ordena top lines por custo desc e limita (ex.: 10)
	sort.Slice(lines, func(i, j int) bool { return lines[i].Cost > lines[j].Cost })
	topLimit := 10
	if len(lines) < topLimit {
		topLimit = len(lines)
	}
	topLines := lines[:topLimit]

	// Monta as categorias ordenadas
	var categories []entity.DataTransferCategoryCost
	for k, v := range categoryTotals {
		if v < 0.001 {
			continue
		}
		categories = append(categories, entity.DataTransferCategoryCost{
			Category: k,
			Cost:     v,
		})
	}
	sort.Slice(categories, func(i, j int) bool { return categories[i].Cost > categories[j].Cost })

	accountID, _ := r.GetAccountID(ctx, profile)

	return entity.DataTransferReport{
		AccountID:   accountID,
		Total:       total,
		Categories:  categories,
		TopLines:    topLines,
		PeriodStart: startDate,
		PeriodEnd:   endDate,
		PeriodName:  periodName,
	}, nil
}

// classifyUsageType classifica um USAGE_TYPE em uma das categorias de data transfer.
// Retorna (categoria, relevante) — relevante = false significa "não é data transfer".
func classifyUsageType(usage string) (string, bool) {
	u := strings.ToLower(usage)

	// NAT Gateway
	if strings.Contains(u, "natgateway-bytes") {
		return "NAT Gateway", true
	}

	// Internet egress: DataTransfer-Out-Bytes (com ou sem prefixo regional, ex: USE2-DataTransfer-Out-Bytes)
	if strings.Contains(u, "datatransfer-out") {
		return "Internet", true
	}

	// Inter-Region: InterRegion keywords
	if strings.Contains(u, "interregion") || strings.Contains(u, "inter-region") {
		return "Inter-Region", true
	}

	// Cross-AZ/Regional: Regional-Bytes
	if strings.Contains(u, "regional-bytes") {
		return "Cross-AZ/Regional", true
	}

	// Outros padrões de "transfer" que não se encaixem acima
	if strings.Contains(u, "datatransfer") || strings.Contains(u, "transfer-bytes") {
		return "Other", true
	}

	// Caso contrário, não consideramos relevante para "data transfer"
	return "Other", false
}

// GetCloudWatchLogGroups lista Log Groups (nome, retenção e tamanho armazenado) por região.
// RetentionDays = 0 significa "Never expire". Ordenação e top-N são tratadas no usecase/export.
func (r *AWSRepositoryImpl) GetCloudWatchLogGroups(ctx context.Context, profile string, regions []string) ([]entity.CloudWatchLogGroupInfo, error) {
	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		result []entity.CloudWatchLogGroupInfo
	)

	for _, region := range regions {
		wg.Add(1)
		go func(rgn string) {
			defer wg.Done()

			clientIntf, err := r.getServiceClient(ctx, profile, rgn, "cloudwatchlogs")
			if err != nil {
				return
			}
			// Se o cliente de cloudwatchlogs não estiver no switch, crie aqui:
			var cwlClient *cloudwatchlogs.Client
			switch v := clientIntf.(type) {
			case *cloudwatchlogs.Client:
				cwlClient = v
			default:
				// Caso o switch não tenha "cloudwatchlogs", criamos aqui como fallback:
				cfg, cfgErr := r.getAWSConfig(ctx, profile)
				if cfgErr != nil {
					return
				}
				cfgRegional := cfg.Copy()
				cfgRegional.Region = rgn
				cwlClient = cloudwatchlogs.NewFromConfig(cfgRegional)
			}

			p := cloudwatchlogs.NewDescribeLogGroupsPaginator(cwlClient, &cloudwatchlogs.DescribeLogGroupsInput{
				Limit: aws.Int32(50),
			})

			for p.HasMorePages() {
				page, err := p.NextPage(ctx)
				if err != nil {
					return
				}
				for _, lg := range page.LogGroups {
					info := entity.CloudWatchLogGroupInfo{
						GroupName: aws.ToString(lg.LogGroupName),
						Region:    rgn,
						StoredBytes: func(b *int64) int64 {
							if b == nil {
								return 0
							}
							return *b
						}(lg.StoredBytes),
						RetentionDays: func(d *int32) int {
							if d == nil {
								return 0
							}
							return int(*d)
						}(lg.RetentionInDays),
					}
					mu.Lock()
					result = append(result, info)
					mu.Unlock()
				}
			}
		}(region)
	}
	wg.Wait()
	return result, nil
}

// GetS3LifecycleStatus coleta status de lifecycle/versioning/intelligent-tiering para todos os buckets,
// além de criptografia padrão, public access block e heurística de exposição pública.
//
// Estratégia:
// - ListBuckets (cliente s3 em us-east-1)
// - Para cada bucket:
//   - GetBucketLocation para descobrir a região (mapa "" -> us-east-1)
//   - Com cliente regional:
//   - GetBucketVersioning (Versioning/MFA Delete)
//   - GetBucketLifecycleConfiguration (regras + Noncurrent + IT via lifecycle)
//   - ListBucketIntelligentTieringConfigurations (IT configs explícitas)
//   - GetBucketEncryption (default encryption SSE-S3/KMS)
//   - GetPublicAccessBlock (flags de PAB)
//   - GetBucketAcl + GetBucketPolicy (heurística de exposição pública)
func (r *AWSRepositoryImpl) GetS3LifecycleStatus(ctx context.Context, profile string) ([]entity.S3BucketLifecycleStatus, error) {
	// Cliente S3 "global" (us-east-1)
	clientIntf, err := r.getServiceClient(ctx, profile, "us-east-1", "s3")
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %w", err)
	}
	s3Global := clientIntf.(*s3.Client)

	// Lista buckets
	listOut, err := s3Global.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to list S3 buckets: %w", err)
	}
	if len(listOut.Buckets) == 0 {
		return nil, nil
	}

	type work struct {
		Bucket string
	}
	jobs := make(chan work, len(listOut.Buckets))
	results := make(chan entity.S3BucketLifecycleStatus, len(listOut.Buckets))

	// Worker pool para evitar throttling: 8 workers
	const workers = 8
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				status := r.inspectBucketLifecycle(ctx, profile, s3Global, j.Bucket)
				results <- status
			}
		}()
	}

	for _, b := range listOut.Buckets {
		if b.Name == nil {
			continue
		}
		jobs <- work{Bucket: *b.Name}
	}
	close(jobs)
	wg.Wait()
	close(results)

	out := make([]entity.S3BucketLifecycleStatus, 0, len(listOut.Buckets))
	for s := range results {
		out = append(out, s)
	}
	return out, nil
}

// inspectBucketLifecycle executa a inspeção detalhada de um bucket.
func (r *AWSRepositoryImpl) inspectBucketLifecycle(ctx context.Context, profile string, s3Global *s3.Client, bucket string) entity.S3BucketLifecycleStatus {
	status := entity.S3BucketLifecycleStatus{
		Bucket:                            bucket,
		Region:                            "us-east-1", // default até apurarmos
		HasLifecycle:                      false,
		LifecycleRulesCount:               0,
		HasNoncurrentLifecycle:            false,
		VersioningEnabled:                 false,
		VersioningMFADelete:               false,
		HasIntelligentTieringCfg:          false,
		HasIntelligentTieringViaLifecycle: false,
		DefaultEncryptionEnabled:          false,
		DefaultEncryptionAlgo:             "",
		DefaultEncryptionKMSKey:           "",
		BlockPublicAcls:                   false,
		BlockPublicPolicy:                 false,
		IgnorePublicAcls:                  false,
		RestrictPublicBuckets:             false,
		IsPublic:                          false,
	}

	// 1) Região do bucket
	if locOut, err := s3Global.GetBucketLocation(ctx, &s3.GetBucketLocationInput{Bucket: &bucket}); err == nil {
		region := "us-east-1"
		if locOut.LocationConstraint != "" {
			region = string(locOut.LocationConstraint)
			if region == "" {
				region = "us-east-1"
			}
		}
		status.Region = region
	}

	// 2) Cliente regional
	clientIntf, err := r.getServiceClient(ctx, profile, status.Region, "s3")
	if err != nil {
		return status
	}
	s3Regional := clientIntf.(*s3.Client)

	// 3) Versioning
	if vOut, err := s3Regional.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: &bucket}); err == nil && vOut != nil {
		status.VersioningEnabled = vOut.Status == s3types.BucketVersioningStatusEnabled
		if vOut.MFADelete == s3types.MFADeleteStatusEnabled {
			status.VersioningMFADelete = true
		}
	}

	// 4) Lifecycle
	if lcOut, err := s3Regional.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{Bucket: &bucket}); err == nil && lcOut != nil {
		if len(lcOut.Rules) > 0 {
			status.HasLifecycle = true
			status.LifecycleRulesCount = len(lcOut.Rules)
			for _, rule := range lcOut.Rules {
				// NoncurrentVersionExpiration ou Transition
				if rule.NoncurrentVersionExpiration != nil && rule.NoncurrentVersionExpiration.NoncurrentDays != nil {
					status.HasNoncurrentLifecycle = true
				}
				if len(rule.NoncurrentVersionTransitions) > 0 {
					status.HasNoncurrentLifecycle = true
				}
				// Intelligent-Tiering via lifecycle transitions
				for _, t := range rule.Transitions {
					if t.StorageClass == s3types.TransitionStorageClassIntelligentTiering {
						status.HasIntelligentTieringViaLifecycle = true
					}
				}
			}
		}
	}

	// 5) Intelligent-Tiering (configurações explícitas de IT)
	if itOut, err := s3Regional.ListBucketIntelligentTieringConfigurations(ctx, &s3.ListBucketIntelligentTieringConfigurationsInput{
		Bucket: &bucket,
	}); err == nil && itOut != nil && len(itOut.IntelligentTieringConfigurationList) > 0 {
		status.HasIntelligentTieringCfg = true
	}

	// 6) Default Encryption
	if encOut, err := s3Regional.GetBucketEncryption(ctx, &s3.GetBucketEncryptionInput{Bucket: &bucket}); err == nil && encOut != nil {
		if encOut.ServerSideEncryptionConfiguration != nil && len(encOut.ServerSideEncryptionConfiguration.Rules) > 0 {
			status.DefaultEncryptionEnabled = true
			rule := encOut.ServerSideEncryptionConfiguration.Rules[0]
			if rule.ApplyServerSideEncryptionByDefault != nil {
				sse := rule.ApplyServerSideEncryptionByDefault
				if sse.SSEAlgorithm != "" {
					status.DefaultEncryptionAlgo = string(sse.SSEAlgorithm)
				}
				if sse.KMSMasterKeyID != nil {
					status.DefaultEncryptionKMSKey = *sse.KMSMasterKeyID
				}
			}
		}
	}

	// 7) Public Access Block (bucket-level) — campos são *bool, converter com aws.ToBool
	if pabOut, err := s3Regional.GetPublicAccessBlock(ctx, &s3.GetPublicAccessBlockInput{Bucket: &bucket}); err == nil && pabOut != nil && pabOut.PublicAccessBlockConfiguration != nil {
		cfg := pabOut.PublicAccessBlockConfiguration
		status.BlockPublicAcls = aws.ToBool(cfg.BlockPublicAcls)
		status.BlockPublicPolicy = aws.ToBool(cfg.BlockPublicPolicy)
		status.IgnorePublicAcls = aws.ToBool(cfg.IgnorePublicAcls)
		status.RestrictPublicBuckets = aws.ToBool(cfg.RestrictPublicBuckets)
	}

	// 8) Heurística de exposição pública (ACL + Policy)
	// 8.1 ACL: grants para AllUsers/AuthenticatedUsers
	if aclOut, err := s3Regional.GetBucketAcl(ctx, &s3.GetBucketAclInput{Bucket: &bucket}); err == nil {
		for _, g := range aclOut.Grants {
			if g.Grantee == nil || g.Grantee.URI == nil || g.Permission == "" {
				continue
			}
			uri := *g.Grantee.URI
			if strings.Contains(uri, "AllUsers") || strings.Contains(uri, "AuthenticatedUsers") {
				status.IsPublic = true
				break
			}
		}
	}
	// 8.2 Policy: Principal="*" + Effect:Allow
	if polOut, err := s3Regional.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{Bucket: &bucket}); err == nil && polOut != nil && polOut.Policy != nil {
		pol := *polOut.Policy
		if (strings.Contains(pol, `"Principal":"*"`) || strings.Contains(pol, `"Principal": "*"`) || strings.Contains(pol, `"AWS":"*"`) || strings.Contains(pol, `"AWS": "*"`)) &&
			(strings.Contains(pol, `"Effect":"Allow"`) || strings.Contains(pol, `"Effect": "Allow"`)) {
			status.IsPublic = true
		}
	}

	return status
}

// GetSavingsPlansSummary consulta cobertura (por serviço) e utilização (total) de Savings Plans.
// Em caso de DataUnavailableException, retorna dados zerados e a flag DataUnavailable = true.
func (r *AWSRepositoryImpl) GetSavingsPlansSummary(ctx context.Context, profile string, timeRange *int, tags []string) (entity.SPSummary, error) {
	client, err := r.getServiceClient(ctx, profile, "", "costexplorer")
	if err != nil {
		return entity.SPSummary{}, err
	}
	ceClient := client.(*costexplorer.Client)

	// Período
	today := time.Now().UTC()
	var startDate, endDate time.Time
	periodName := "Current month's SP"
	if timeRange != nil && *timeRange > 0 {
		endDate = today
		startDate = today.AddDate(0, 0, -(*timeRange))
		periodName = fmt.Sprintf("Current %d days SP", *timeRange)
	} else {
		startDate = time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
		endDate = today
		if startDate.Day() == endDate.Day() && startDate.Month() == endDate.Month() && startDate.Year() == endDate.Year() {
			endDate = endDate.AddDate(0, 0, 1)
		}
	}

	// Relatório base com DataUnavailable
	accountID, _ := r.GetAccountID(ctx, profile)
	baseSummary := entity.SPSummary{
		AccountID:       accountID,
		PeriodStart:     startDate,
		PeriodEnd:       endDate,
		PeriodName:      periodName,
		DataUnavailable: false,
	}

	filter, _ := parseTagFilter(tags)

	// 1) Cobertura por serviço
	coverageInput := &costexplorer.GetSavingsPlansCoverageInput{
		TimePeriod: &ceTypes.DateInterval{
			Start: aws.String(startDate.Format("2006-01-02")),
			End:   aws.String(endDate.Format("2006-01-02")),
		},
		Granularity: ceTypes.GranularityMonthly,
		GroupBy: []ceTypes.GroupDefinition{
			{Type: ceTypes.GroupDefinitionTypeDimension, Key: aws.String("SERVICE")},
		},
	}
	if filter != nil {
		coverageInput.Filter = filter
	}

	coverageOut, err := ceClient.GetSavingsPlansCoverage(ctx, coverageInput)
	if err != nil && filter != nil {
		coverageInput.Filter = nil
		coverageOut, err = ceClient.GetSavingsPlansCoverage(ctx, coverageInput)
	}
	if err != nil {
		if r.isCEDataUnavailable(err) {
			baseSummary.DataUnavailable = true
			return baseSummary, nil // Retorna zerado com aviso
		}
		return entity.SPSummary{}, fmt.Errorf("GetSavingsPlansCoverage failed: %w", err)
	}

	var perService []entity.ServiceCoverage
	var totalCoveredCost, totalOnDemandCost float64

	if len(coverageOut.SavingsPlansCoverages) > 0 {
		for _, item := range coverageOut.SavingsPlansCoverages {
			svc := ""
			if item.Attributes != nil {
				if v, ok := item.Attributes["SERVICE"]; ok {
					svc = v
				}
			}
			var covered, ondemand, pct float64
			if item.Coverage != nil {
				if item.Coverage.SpendCoveredBySavingsPlans != nil {
					covered, _ = strconv.ParseFloat(*item.Coverage.SpendCoveredBySavingsPlans, 64)
				}
				if item.Coverage.OnDemandCost != nil {
					ondemand, _ = strconv.ParseFloat(*item.Coverage.OnDemandCost, 64)
				}
				if item.Coverage.CoveragePercentage != nil {
					pct, _ = strconv.ParseFloat(*item.Coverage.CoveragePercentage, 64)
				} else if (covered + ondemand) > 0.001 {
					pct = (covered / (covered + ondemand)) * 100.0
				}
			}
			if covered+ondemand > 0.001 {
				perService = append(perService, entity.ServiceCoverage{
					Service:         svc,
					CoveragePercent: pct,
					CoveredCost:     covered,
					OnDemandCost:    ondemand,
				})
				totalCoveredCost += covered
				totalOnDemandCost += ondemand
			}
		}
	}

	sort.Slice(perService, func(i, j int) bool { return perService[i].OnDemandCost > perService[j].OnDemandCost })

	overallCoveragePercent := 0.0
	if (totalCoveredCost + totalOnDemandCost) > 0.001 {
		overallCoveragePercent = (totalCoveredCost / (totalCoveredCost + totalOnDemandCost)) * 100.0
	}
	baseSummary.CoveragePercent = overallCoveragePercent
	baseSummary.PerServiceCoverage = perService

	// 2) Utilização (agregado)
	utilInput := &costexplorer.GetSavingsPlansUtilizationInput{
		TimePeriod: &ceTypes.DateInterval{
			Start: aws.String(startDate.Format("2006-01-02")),
			End:   aws.String(endDate.Format("2006-01-02")),
		},
		Granularity: ceTypes.GranularityMonthly,
	}
	if filter != nil {
		utilInput.Filter = filter
	}

	utilOut, err := ceClient.GetSavingsPlansUtilization(ctx, utilInput)
	if err != nil && filter != nil {
		utilInput.Filter = nil
		utilOut, err = ceClient.GetSavingsPlansUtilization(ctx, utilInput)
	}
	if err != nil {
		if r.isCEDataUnavailable(err) {
			baseSummary.DataUnavailable = true
			return baseSummary, nil
		}
		return entity.SPSummary{}, fmt.Errorf("GetSavingsPlansUtilization failed: %w", err)
	}

	var sumTotalCommit, sumUsedCommit, sumUnusedCommit float64
	if len(utilOut.SavingsPlansUtilizationsByTime) > 0 {
		for _, by := range utilOut.SavingsPlansUtilizationsByTime {
			agg := by.Utilization
			if agg == nil {
				continue
			}
			if agg.TotalCommitment != nil {
				v, _ := strconv.ParseFloat(*agg.TotalCommitment, 64)
				sumTotalCommit += v
			}
			if agg.UsedCommitment != nil {
				v, _ := strconv.ParseFloat(*agg.UsedCommitment, 64)
				sumUsedCommit += v
			}
			if agg.UnusedCommitment != nil {
				v, _ := strconv.ParseFloat(*agg.UnusedCommitment, 64)
				sumUnusedCommit += v
			}
		}
	}

	utilPct := 0.0
	if sumTotalCommit > 0.001 {
		utilPct = (sumUsedCommit / sumTotalCommit) * 100.0
	}

	baseSummary.UtilizationPercent = utilPct
	baseSummary.TotalCommitment = sumTotalCommit
	baseSummary.UsedCommitment = sumUsedCommit
	baseSummary.UnusedCommitment = sumUnusedCommit

	return baseSummary, nil
}

// GetReservationSummary consulta cobertura (por família de instância) e utilização (total) de Reserved Instances.
// Em caso de ValidationException ou DataUnavailableException, retorna dados zerados e a flag DataUnavailable = true.
func (r *AWSRepositoryImpl) GetReservationSummary(ctx context.Context, profile string, timeRange *int, tags []string) (entity.RISummary, error) {
	client, err := r.getServiceClient(ctx, profile, "", "costexplorer")
	if err != nil {
		return entity.RISummary{}, err
	}
	ceClient := client.(*costexplorer.Client)

	// Período
	today := time.Now().UTC()
	var startDate, endDate time.Time
	periodName := "Current month's RI"
	if timeRange != nil && *timeRange > 0 {
		endDate = today
		startDate = today.AddDate(0, 0, -(*timeRange))
		periodName = fmt.Sprintf("Current %d days RI", *timeRange)
	} else {
		startDate = time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
		endDate = today
		if startDate.Day() == endDate.Day() && startDate.Month() == endDate.Month() && startDate.Year() == endDate.Year() {
			endDate = endDate.AddDate(0, 0, 1)
		}
	}

	accountID, _ := r.GetAccountID(ctx, profile)
	baseSummary := entity.RISummary{
		AccountID:       accountID,
		PeriodStart:     startDate,
		PeriodEnd:       endDate,
		PeriodName:      periodName,
		DataUnavailable: false,
	}

	filter, _ := parseTagFilter(tags)

	// 1) Cobertura — tentar por INSTANCE_TYPE_FAMILY (suportado)
	riCovInput := &costexplorer.GetReservationCoverageInput{
		TimePeriod: &ceTypes.DateInterval{
			Start: aws.String(startDate.Format("2006-01-02")),
			End:   aws.String(endDate.Format("2006-01-02")),
		},
		Granularity: ceTypes.GranularityMonthly,
		GroupBy: []ceTypes.GroupDefinition{
			{Type: ceTypes.GroupDefinitionTypeDimension, Key: aws.String("INSTANCE_TYPE_FAMILY")},
		},
	}
	if filter != nil {
		riCovInput.Filter = filter
	}

	riCovOut, err := ceClient.GetReservationCoverage(ctx, riCovInput)
	if err != nil {
		if filter != nil {
			riCovInput.Filter = nil
			riCovOut, err = ceClient.GetReservationCoverage(ctx, riCovInput)
		}
		if err != nil && (r.isCEValidationException(err) || r.isCEDataUnavailable(err)) {
			riCovInput.GroupBy = nil
			riCovOut, err = ceClient.GetReservationCoverage(ctx, riCovInput)
		}
	}
	if err != nil {
		if r.isCEDataUnavailable(err) || r.isCEValidationException(err) {
			baseSummary.DataUnavailable = true
			return baseSummary, nil
		}
		return entity.RISummary{}, fmt.Errorf("GetReservationCoverage failed: %w", err)
	}

	var perFamily []entity.ServiceCoverage
	overallCoveragePercent := 0.0

	if riCovOut != nil && len(riCovOut.CoveragesByTime) > 0 {
		for _, byTime := range riCovOut.CoveragesByTime {
			if len(byTime.Groups) > 0 {
				for _, grp := range byTime.Groups {
					label := ""
					if grp.Attributes != nil {
						if v, ok := grp.Attributes["INSTANCE_TYPE_FAMILY"]; ok {
							label = v
						}
					}
					var pct, onDemandHrs float64
					if grp.Coverage != nil && grp.Coverage.CoverageHours != nil {
						if grp.Coverage.CoverageHours.CoverageHoursPercentage != nil {
							pct, _ = strconv.ParseFloat(*grp.Coverage.CoverageHours.CoverageHoursPercentage, 64)
						}
						if grp.Coverage.CoverageHours.OnDemandHours != nil {
							onDemandHrs, _ = strconv.ParseFloat(*grp.Coverage.CoverageHours.OnDemandHours, 64)
						}
					}
					perFamily = append(perFamily, entity.ServiceCoverage{
						Service:         label,
						CoveragePercent: pct,
						OnDemandCost:    onDemandHrs,
					})
				}
			}
			if byTime.Total != nil && byTime.Total.CoverageHours != nil && byTime.Total.CoverageHours.CoverageHoursPercentage != nil {
				if v, e := strconv.ParseFloat(*byTime.Total.CoverageHours.CoverageHoursPercentage, 64); e == nil {
					overallCoveragePercent = v
				}
			}
		}
		sort.Slice(perFamily, func(i, j int) bool { return perFamily[i].OnDemandCost > perFamily[j].OnDemandCost })
	}
	baseSummary.CoveragePercent = overallCoveragePercent
	baseSummary.PerServiceCoverage = perFamily

	// 2) Utilização
	riUtilInput := &costexplorer.GetReservationUtilizationInput{
		TimePeriod: &ceTypes.DateInterval{
			Start: aws.String(startDate.Format("2006-01-02")),
			End:   aws.String(endDate.Format("2006-01-02")),
		},
		Granularity: ceTypes.GranularityMonthly,
	}
	if filter != nil {
		riUtilInput.Filter = filter
	}
	riUtilOut, err := ceClient.GetReservationUtilization(ctx, riUtilInput)
	if err != nil && filter != nil {
		riUtilInput.Filter = nil
		riUtilOut, err = ceClient.GetReservationUtilization(ctx, riUtilInput)
	}
	var utilPct, usedHours, unusedHours, totalHours float64
	if err != nil {
		if r.isCEDataUnavailable(err) {
			baseSummary.DataUnavailable = true
			return baseSummary, nil
		}
		return entity.RISummary{}, fmt.Errorf("GetReservationUtilization failed: %w", err)
	} else if riUtilOut != nil && riUtilOut.Total != nil {
		if riUtilOut.Total.UtilizationPercentage != nil {
			utilPct, _ = strconv.ParseFloat(*riUtilOut.Total.UtilizationPercentage, 64)
		}
		if riUtilOut.Total.TotalActualHours != nil {
			usedHours, _ = strconv.ParseFloat(*riUtilOut.Total.TotalActualHours, 64)
		}
		if riUtilOut.Total.UnusedHours != nil {
			unusedHours, _ = strconv.ParseFloat(*riUtilOut.Total.UnusedHours, 64)
		}
		if riUtilOut.Total.PurchasedHours != nil {
			totalHours, _ = strconv.ParseFloat(*riUtilOut.Total.PurchasedHours, 64)
		}
	}
	if totalHours < 0.001 && (usedHours+unusedHours) > 0.001 {
		totalHours = usedHours + unusedHours
	}
	if utilPct < 0.001 && totalHours > 0.001 {
		utilPct = (usedHours / totalHours) * 100.0
	}

	baseSummary.UtilizationPercent = utilPct
	baseSummary.UsedHours = usedHours
	baseSummary.UnusedHours = unusedHours
	baseSummary.TotalReservedHours = totalHours

	return baseSummary, nil
}

// isCEDataUnavailable identifica o erro "DataUnavailableException" do Cost Explorer.
func (r *AWSRepositoryImpl) isCEDataUnavailable(err error) bool {
	if err == nil {
		return false
	}
	// Checagem defensiva por substring é a mais estável no SDK v2 independente do tipo concreto.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "dataunavailableexception") || strings.Contains(msg, "data unavailable")
}

// isCEValidationException identifica "ValidationException" do Cost Explorer.
func (r *AWSRepositoryImpl) isCEValidationException(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "validationexception")
}

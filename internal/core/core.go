package core

import (
	"runtime"
	"sort"
	"sync"
	"time"

	"slices"

	"github.com/NamiraNet/namira-core/internal/core/checker"
	"github.com/NamiraNet/namira-core/internal/core/parser"
)

type CheckResultStatusType string

const (
	CheckResultStatusSuccess     CheckResultStatusType = "success"
	CheckResultStatusUnavailable CheckResultStatusType = "unavailable"
	CheckResultStatusError       CheckResultStatusType = "error"
)

const (
	DefaultCheckTimeout = 10 * time.Second
	DefaultCheckServer  = "1.1.1.1"
	DefaultCheckPort    = 443
)

type Config interface {
	MarshalJSON() ([]byte, error)
}

type CheckResult struct {
	Status         CheckResultStatusType
	Protocol       string
	Raw            string
	RealDelay      time.Duration
	Remark         string
	Server         string
	CountryCode    string
	Error          string
	CheckerNodeTag []string // Array of worker tags that successfully processed this result
}

type Core struct {
	checker             checker.ConfigChecker
	parser              *parser.Parser
	remarkTemplate      RemarkTemplate
	maxConcurrentChecks int
}

type CoreOpts struct {
	CheckTimeout       time.Duration
	CheckServer        string
	CheckPort          uint32
	CheckMaxConcurrent int
	RemarkTemplate     *RemarkTemplate
}

func calculateMaxConcurrent() int {
	numCPU := runtime.NumCPU()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	availableMemMB := memStats.Sys / 1024 / 1024

	basePerCPU := 500
	cpuBasedLimit := numCPU * basePerCPU

	memoryBasedLimit := int(availableMemMB * 1024)
	maxConcurrent := min(memoryBasedLimit, cpuBasedLimit)

	minLimit := 100
	maxLimit := getSystemFDLimit()

	// reserve FDs for other operations (80% of available)
	maxLimit = int(float64(maxLimit) * 0.8)

	if maxConcurrent < minLimit {
		maxConcurrent = minLimit
	} else if maxConcurrent > maxLimit {
		maxConcurrent = maxLimit
	}

	return maxConcurrent
}

func NewCore(opts ...CoreOpts) *Core {
	if len(opts) == 0 {
		opts = append(opts, CoreOpts{
			CheckTimeout:       10 * time.Second,
			CheckServer:        "1.1.1.1",
			CheckPort:          80,
			CheckMaxConcurrent: calculateMaxConcurrent(),
		})
	}

	remarkTemplate := DefaultRemarkTemplate()
	if opts[0].RemarkTemplate != nil {
		remarkTemplate = *opts[0].RemarkTemplate
	}

	if opts[0].CheckMaxConcurrent == 0 {
		opts[0].CheckMaxConcurrent = calculateMaxConcurrent()
	}
	if opts[0].CheckTimeout == 0 {
		opts[0].CheckTimeout = DefaultCheckTimeout
	}
	if opts[0].CheckServer == "" {
		opts[0].CheckServer = DefaultCheckServer
	}
	if opts[0].CheckPort == 0 {
		opts[0].CheckPort = DefaultCheckPort
	}

	return &Core{
		checker:             checker.NewV2RayConfigChecker(opts[0].CheckTimeout, opts[0].CheckServer, opts[0].CheckPort),
		parser:              parser.NewParser(),
		maxConcurrentChecks: opts[0].CheckMaxConcurrent,
		remarkTemplate:      remarkTemplate,
	}
}

func (c *Core) CheckConfigs(configs []string) <-chan CheckResult {
	results := make(chan CheckResult)
	var wg sync.WaitGroup
	sem := make(chan struct{}, c.maxConcurrentChecks)
	go func() {
		for i, config := range configs {
			wg.Add(1)
			sem <- struct{}{} // Acquire semaphore
			go func(index int, cfg string) {
				defer wg.Done()
				defer func() { <-sem }() // Release semaphore

				result := CheckResult{
					Status:         CheckResultStatusSuccess,
					Raw:            cfg,
					CheckerNodeTag: []string{"local"}, // Default tag for local processing
				}

				parsed, err := c.parser.Parse(cfg)

				if err != nil {
					result.Status = CheckResultStatusError
					result.Error = err.Error()
					results <- result
					return
				}

				delay, err := c.checker.CheckConfig(parsed)
				c.FillCheckResult(&result)

				if err != nil {
					result.Status = CheckResultStatusError
					result.Error = err.Error()
				} else {
					result.RealDelay = delay
				}

				results <- result
			}(i, config)
		}
		wg.Wait()
		close(results)
	}()

	return results
}

func (c *Core) CheckConfigsList(configs []string) []CheckResult {
	results := make([]CheckResult, 0)
	for result := range c.CheckConfigs(configs) {
		results = append(results, result)
	}
	return results
}

func (c *Core) SortCheckResultList(results []CheckResult) []CheckResult {
	results = slices.Clone(results)
	sort.Slice(results, func(i, j int) bool {
		if results[i].Status != results[j].Status {
			return results[i].Status == CheckResultStatusSuccess
		}
		return results[i].RealDelay < results[j].RealDelay
	})
	return results
}

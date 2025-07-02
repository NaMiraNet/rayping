package github

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/NamiraNet/namira-core/internal/core"
	"github.com/NamiraNet/namira-core/internal/crypto"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	FILENAME    = "results.txt"
	CLONE_DEPTH = 1
	FILE_PERMS  = 0644
	BOT_NAME    = "Namira Bot"
	BOT_EMAIL   = "namiranet@proton.me"
	REMOTE_NAME = "origin"
)

type Updater struct {
	auth          *ssh.PublicKeys
	redisClient   *redis.Client
	repoOwner     string
	repoName      string
	repoURL       string
	encryptionKey []byte
	logger        *zap.Logger
	workDir       string
}

type ScanResult struct {
	JobID     string             `json:"job_id"`
	Results   []core.CheckResult `json:"results"`
	Timestamp time.Time          `json:"timestamp"`
}

type JSONResult struct {
	JobID     string             `json:"job_id"`
	Timestamp time.Time          `json:"timestamp"`
	Results   []JSONConfigResult `json:"results"`
}

type JSONConfigResult struct {
	Delay       int64  `json:"delay_ms"`
	Status      string `json:"status"`
	Protocol    string `json:"protocol"`
	RawConfig   string `json:"raw_config"`
	CountryCode string `json:"country_code"`
	Remark      string `json:"remark"`
	Server      string `json:"server"`
}

func NewUpdater(log *zap.Logger, sshKeyPath string, redisClient *redis.Client, repoOwner, repoName string, encryptionKey []byte) (*Updater, error) {
	if _, err := os.Stat(sshKeyPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("SSH key file not found: %s", sshKeyPath)
	}

	// Setup SSH authentication
	auth, err := ssh.NewPublicKeysFromFile("git", sshKeyPath, "")
	if err != nil {
		return nil, fmt.Errorf("failed to load SSH key: %w", err)
	}

	return &Updater{
		auth:          auth,
		redisClient:   redisClient,
		repoOwner:     repoOwner,
		repoName:      repoName,
		repoURL:       fmt.Sprintf("git@github.com:%s/%s.git", repoOwner, repoName),
		encryptionKey: encryptionKey,
		logger:        log,
		workDir:       fmt.Sprintf("/tmp/namira-core-updater-%s-%s", repoOwner, repoName),
	}, nil
}

// HealthCheck tests SSH connectivity to GitHub
func (u *Updater) HealthCheck() error {
	tempDir := u.workDir + "-healthcheck"
	defer os.RemoveAll(tempDir)

	_, err := git.PlainClone(tempDir, false, &git.CloneOptions{
		URL:   u.repoURL,
		Auth:  u.auth,
		Depth: CLONE_DEPTH,
	})
	return err
}

func (u *Updater) processScanResultsCommon(jobID string, merge bool, taskType string) error {
	resultsData, err := u.fetchResults(jobID)
	if err != nil {
		return fmt.Errorf("fetch results failed: %w", err)
	}

	results, err := u.prepareContent(resultsData)
	if err != nil {
		return fmt.Errorf("prepare content failed: %w", err)
	}

	if err := u.updateFileViaGit(jobID, results, merge); err != nil {
		return fmt.Errorf("git update failed: %w", err)
	}

	u.logger.Info("Successfully updated results on GitHub",
		zap.String("job_id", jobID),
		zap.String("task_type", taskType),
		zap.String("repo", fmt.Sprintf("%s/%s", u.repoOwner, u.repoName)))

	return nil
}

func (u *Updater) ProcessScanResults(jobID string) error {
	return u.processScanResultsCommon(jobID, true, "scan")
}

func (u *Updater) ProcessRefreshResults(jobID string) error {
	return u.processScanResultsCommon(jobID, false, "refresh")
}

func (u *Updater) fetchResults(jobID string) ([]byte, error) {
	resultsKey := fmt.Sprintf("scan_results:%s", jobID)
	resultsData, err := u.redisClient.Get(context.Background(), resultsKey).Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch results from Redis: %w", err)
	}
	return resultsData, nil
}

func (u *Updater) prepareContent(resultsData []byte) (JSONResult, error) {
	var scanResult ScanResult
	if err := json.Unmarshal(resultsData, &scanResult); err != nil {
		return JSONResult{}, fmt.Errorf("failed to unmarshal scan results: %w", err)
	}
	return formatResultsJSON(scanResult), nil
}

func (u *Updater) updateFileViaGit(jobID string, current JSONResult, merge bool) error {
	os.RemoveAll(u.workDir)
	defer os.RemoveAll(u.workDir)

	repo, err := git.PlainClone(u.workDir, false, &git.CloneOptions{
		URL:   u.repoURL,
		Auth:  u.auth,
		Depth: CLONE_DEPTH,
	})
	if err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	filePath := filepath.Join(u.workDir, FILENAME)

	if merge {
		if err := u.mergeExistingContent(filePath, &current); err != nil {
			u.logger.Warn("Failed to merge existing content.", zap.Error(err))
		}
	}

	if err := u.writeEncryptedContent(filePath, current); err != nil {
		return err
	}

	sort.Slice(current.Results, func(i, j int) bool {
		return current.Results[i].Delay < current.Results[j].Delay
	})

	return u.commitAndPush(repo, jobID)
}

func (u *Updater) hashConfig(config string) string {
	sum := sha256.Sum256([]byte(config))
	return hex.EncodeToString(sum[:])
}

func (u *Updater) mergeExistingContent(filePath string, current *JSONResult) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	decoded, err := base64.StdEncoding.DecodeString(string(content))
	if err != nil {
		return err
	}

	decrypted, err := crypto.Decrypt(decoded, u.encryptionKey)
	if err != nil {
		return err
	}

	var existing JSONResult
	if err := json.Unmarshal(decrypted, &existing); err != nil {
		return err
	}

	if len(existing.Results) == 0 {
		return nil
	}

	configMap := make(map[string]struct{}, len(current.Results))
	for _, result := range current.Results {
		configMap[u.hashConfig(result.RawConfig)] = struct{}{}
	}

	for _, result := range existing.Results {
		if _, exists := configMap[u.hashConfig(result.RawConfig)]; !exists {
			current.Results = append(current.Results, result)
		}
	}

	return nil
}

func (u *Updater) writeEncryptedContent(filePath string, content JSONResult) error {
	jsonContent, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("marshal content: %w", err)
	}

	encrypted, err := crypto.Encrypt(jsonContent, u.encryptionKey)
	if err != nil {
		return fmt.Errorf("encrypt content: %w", err)
	}

	return os.WriteFile(filePath, []byte(base64.StdEncoding.EncodeToString(encrypted)), FILE_PERMS)
}

func (u *Updater) commitAndPush(repo *git.Repository, jobID string) error {
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	if _, err := worktree.Add(FILENAME); err != nil {
		return fmt.Errorf("failed to add file: %w", err)
	}

	_, err = worktree.Commit(fmt.Sprintf("🤖 Update scan results - Job %s", jobID), &git.CommitOptions{
		Author: &object.Signature{
			Name:  BOT_NAME,
			Email: BOT_EMAIL,
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	if err := repo.Push(&git.PushOptions{
		RemoteName: REMOTE_NAME,
		Auth:       u.auth,
	}); err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}
	return nil
}

func (u *Updater) GetCurrentConfigsViaHTTP() ([]string, error) {
	rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/%s",
		u.repoOwner, u.repoName, FILENAME)

	u.logger.Debug("Fetching configs via HTTP", zap.String("url", rawURL))

	req, err := http.NewRequest(http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("User-Agent", "namira-core/1.1.0")
	req.Header.Set("Accept", "text/plain")

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		u.logger.Info("No existing config file found, starting fresh")
		return []string{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: %d %s", resp.StatusCode, resp.Status)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return u.decryptAndParseConfigs(content)
}

func (u *Updater) decryptAndParseConfigs(content []byte) ([]string, error) {
	decoded, err := base64.StdEncoding.DecodeString(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to decode content: %w", err)
	}

	decrypted, err := crypto.Decrypt(decoded, u.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt content: %w", err)
	}

	var existing JSONResult
	if err := json.Unmarshal(decrypted, &existing); err != nil {
		return nil, fmt.Errorf("failed to unmarshal content: %w", err)
	}

	configs := make([]string, len(existing.Results))
	for i, result := range existing.Results {
		configs[i] = result.RawConfig
	}

	u.logger.Info("Successfully fetched configs", zap.Int("count", len(configs)))
	return configs, nil
}

func (u *Updater) GetCurrentConfigsViaGit() ([]string, error) {
	os.RemoveAll(u.workDir)
	defer os.RemoveAll(u.workDir)

	_, err := git.PlainClone(u.workDir, false, &git.CloneOptions{
		URL:   u.repoURL,
		Auth:  u.auth,
		Depth: CLONE_DEPTH,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	content, err := os.ReadFile(filepath.Join(u.workDir, FILENAME))
	if err != nil {
		return []string{}, nil
	}

	return u.decryptAndParseConfigs(content)
}

func (u *Updater) GetCurrentConfigs() ([]string, error) {
	configs, err := u.GetCurrentConfigsViaHTTP()
	if err != nil {
		u.logger.Warn("HTTP fetch failed, falling back to Git clone", zap.Error(err))
		return u.GetCurrentConfigsViaGit()
	}
	return configs, nil
}

func formatResultsJSON(scanResult ScanResult) JSONResult {
	results := make([]JSONConfigResult, 0, len(scanResult.Results))
	for _, result := range scanResult.Results {
		if result.Status == core.CheckResultStatusSuccess {
			results = append(results, JSONConfigResult{
				Status:      string(result.Status),
				Delay:       result.RealDelay.Milliseconds(),
				Protocol:    result.Protocol,
				RawConfig:   result.Raw,
				CountryCode: result.CountryCode,
				Remark:      result.Remark,
				Server:      result.Server,
			})
		}
	}

	return JSONResult{
		JobID:     scanResult.JobID,
		Timestamp: scanResult.Timestamp,
		Results:   results,
	}
}

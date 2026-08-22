package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mywhoosh2garmin/garmin"
	"mywhoosh2garmin/mywhoosh"
)

const (
	myWhooshEmailEnv    = "MYWHOOSH_EMAIL"
	myWhooshPasswordEnv = "MYWHOOSH_PASSWORD"
	garminEmailEnv      = "GARMIN_EMAIL"
	garminPasswordEnv   = "GARMIN_PASSWORD"
)

// syncedTracker records uploads so repeated runs do not upload the same
// MyWhoosh activity again. It is stored in ~/.mywhoosh2garmin/synced.json.
type syncedTracker struct {
	mu       sync.Mutex
	path     string
	uploaded map[string]string // activity ID -> upload timestamp
}

func appConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".mywhoosh2garmin"), nil
}

func newSyncedTracker(dir string) (*syncedTracker, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}

	tracker := &syncedTracker{
		path:     filepath.Join(dir, "synced.json"),
		uploaded: make(map[string]string),
	}

	data, err := os.ReadFile(tracker.path)
	if errors.Is(err, os.ErrNotExist) {
		return tracker, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read sync state: %w", err)
	}
	if err := json.Unmarshal(data, &tracker.uploaded); err != nil {
		return nil, fmt.Errorf("parse sync state: %w", err)
	}
	if tracker.uploaded == nil {
		tracker.uploaded = make(map[string]string)
	}
	return tracker, nil
}

func (tracker *syncedTracker) IsSynced(activityID string) bool {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	_, ok := tracker.uploaded[activityID]
	return ok
}

func (tracker *syncedTracker) MarkSynced(activityID string) error {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	tracker.uploaded[activityID] = time.Now().Format(time.RFC3339)
	data, err := json.MarshalIndent(tracker.uploaded, "", "  ")
	if err != nil {
		return fmt.Errorf("encode sync state: %w", err)
	}
	if err := os.WriteFile(tracker.path, data, 0o600); err != nil {
		return fmt.Errorf("write sync state: %w", err)
	}
	return nil
}

type credentials struct {
	myWhooshEmail    string
	myWhooshPassword string
	garminEmail      string
	garminPassword   string
}

func credentialsFromEnv() (credentials, error) {
	creds := credentials{
		myWhooshEmail:    os.Getenv(myWhooshEmailEnv),
		myWhooshPassword: os.Getenv(myWhooshPasswordEnv),
		garminEmail:      os.Getenv(garminEmailEnv),
		garminPassword:   os.Getenv(garminPasswordEnv),
	}

	missing := make([]string, 0, 4)
	for _, variable := range []struct {
		name  string
		value string
	}{
		{myWhooshEmailEnv, creds.myWhooshEmail},
		{myWhooshPasswordEnv, creds.myWhooshPassword},
		{garminEmailEnv, creds.garminEmail},
		{garminPasswordEnv, creds.garminPassword},
	} {
		if strings.TrimSpace(variable.value) == "" {
			missing = append(missing, variable.name)
		}
	}
	if len(missing) > 0 {
		return credentials{}, fmt.Errorf("missing required environment variable(s): %s", strings.Join(missing, ", "))
	}
	return creds, nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sync failed:", err)
		os.Exit(1)
	}
}

func run() error {
	creds, err := credentialsFromEnv()
	if err != nil {
		return err
	}

	stateDir, err := appConfigDir()
	if err != nil {
		return err
	}
	tracker, err := newSyncedTracker(stateDir)
	if err != nil {
		return err
	}

	myWhooshClient := mywhoosh.NewClient(stateDir)
	if err := myWhooshClient.Resume(); err != nil {
		fmt.Println("Logging in to MyWhoosh...")
		if err := myWhooshClient.Login(creds.myWhooshEmail, creds.myWhooshPassword); err != nil {
			return fmt.Errorf("MyWhoosh login: %w", err)
		}
	} else {
		fmt.Println("MyWhoosh session resumed.")
	}

	activities, err := myWhooshClient.GetTodayActivities(time.Now())
	if err != nil {
		fmt.Println("MyWhoosh session may have expired; logging in again...")
		if loginErr := myWhooshClient.Login(creds.myWhooshEmail, creds.myWhooshPassword); loginErr != nil {
			return fmt.Errorf("fetch today's MyWhoosh activities: %w (login retry: %v)", err, loginErr)
		}
		activities, err = myWhooshClient.GetTodayActivities(time.Now())
		if err != nil {
			return fmt.Errorf("fetch today's MyWhoosh activities: %w", err)
		}
	}

	if len(activities) == 0 {
		fmt.Println("No MyWhoosh activities found for today.")
		return nil
	}
	fmt.Printf("Found %d MyWhoosh activit%s for today.\n", len(activities), plural(len(activities), "y", "ies"))

	garminClient := garmin.NewClient(stateDir)
	if err := garminClient.Resume(); err != nil {
		fmt.Println("Logging in to Garmin Connect...")
		if err := garminClient.Login(creds.garminEmail, creds.garminPassword); err != nil {
			return fmt.Errorf("Garmin login: %w", err)
		}
	} else {
		fmt.Println("Garmin session resumed.")
	}

	tempDir, err := os.MkdirTemp("", "mywhoosh2garmin-*")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	uploaded, skipped := 0, 0
	var failures []string
	for index, activity := range activities {
		if tracker.IsSynced(activity.ID) {
			fmt.Printf("[%d/%d] Skipping %s: already synced.\n", index+1, len(activities), activity.DisplayName())
			skipped++
			continue
		}

		fmt.Printf("[%d/%d] Syncing %s (%s)...\n", index+1, len(activities), activity.DisplayName(), activity.FormattedDate())
		if err := syncActivity(myWhooshClient, garminClient, tracker, activity, tempDir, index); err != nil {
			if strings.Contains(err.Error(), "duplicate") {
				fmt.Println("  Already in Garmin Connect.")
				if markErr := tracker.MarkSynced(activity.ID); markErr != nil {
					failures = append(failures, fmt.Sprintf("%s: save sync state: %v", activity.DisplayName(), markErr))
					continue
				}
				skipped++
				continue
			}
			failures = append(failures, fmt.Sprintf("%s: %v", activity.DisplayName(), err))
			fmt.Printf("  Failed: %v\n", err)
			continue
		}

		uploaded++
		fmt.Println("  Uploaded to Garmin Connect.")
	}

	fmt.Printf("Done: %d uploaded, %d skipped.\n", uploaded, skipped)
	if len(failures) > 0 {
		return fmt.Errorf("%d activit%s failed: %s", len(failures), plural(len(failures), "y", "ies"), strings.Join(failures, "; "))
	}
	return nil
}

func syncActivity(myWhooshClient *mywhoosh.Client, garminClient *garmin.Client, tracker *syncedTracker, activity mywhoosh.Activity, tempDir string, index int) error {
	fileID := activity.ActivityFileID
	if fileID == "" {
		fileID = activity.ID
	}

	fitData, err := myWhooshClient.DownloadFitFile(fileID)
	if err != nil {
		return fmt.Errorf("download FIT file: %w", err)
	}

	inputPath := filepath.Join(tempDir, fmt.Sprintf("%d-input.fit", index))
	outputPath := filepath.Join(tempDir, fmt.Sprintf("%d-fixed.fit", index))
	if err := os.WriteFile(inputPath, fitData, 0o600); err != nil {
		return fmt.Errorf("write FIT file: %w", err)
	}
	if err := fixFitFile(inputPath, outputPath); err != nil {
		return fmt.Errorf("fix FIT file: %w", err)
	}
	if err := garminClient.UploadFIT(outputPath); err != nil {
		return fmt.Errorf("upload FIT file: %w", err)
	}
	if err := tracker.MarkSynced(activity.ID); err != nil {
		return err
	}
	return nil
}

func plural(count int, singularSuffix, pluralSuffix string) string {
	if count == 1 {
		return singularSuffix
	}
	return pluralSuffix
}

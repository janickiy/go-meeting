package integration_test

import "testing"

func TestFoldersBrowserHarness(t *testing.T) {
	runPersonalBrowserHarness(t, "RECORDER_FOLDER_BROWSER_FIXTURE", "meetrix-folders-", true)
}

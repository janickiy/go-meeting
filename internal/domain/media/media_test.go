package media

import "testing"

// TestPolicyBlocksAudioRelabelledAsScreen проверяет сценарий «политика блокирует звук смена метки как экран», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestPolicyBlocksAudioRelabelledAsScreen(t *testing.T) {
	policy := ParticipantPolicy{Version: 1, MicrophoneBlocked: true}
	if policy.Allows(SourceMicrophone) || policy.Allows(SourceAudioScreen) {
		t.Fatal("muted participant can publish audio")
	}
	if !policy.Allows(SourceCamera) || !policy.Allows(SourceVideoScreen) {
		t.Fatal("audio mute unexpectedly blocks video")
	}
	policy.ScreenBlocked = true
	if policy.Allows(SourceVideoScreen) || policy.Allows(SourceAudioScreen) {
		t.Fatal("screen block can be bypassed")
	}
}

// TestPolicyBlocksVideoRelabelledAsScreen проверяет сценарий «политика блокирует видео смена метки как экран», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestPolicyBlocksVideoRelabelledAsScreen(t *testing.T) {
	policy := ParticipantPolicy{Version: 1, CameraBlocked: true}
	if policy.Allows(SourceCamera) || policy.Allows(SourceVideoScreen) || policy.Allows(SourceAudioScreen) {
		t.Fatal("video-disabled participant can relabel a camera as screen")
	}
	if !policy.Allows(SourceMicrophone) {
		t.Fatal("video block unexpectedly muted microphone")
	}
}

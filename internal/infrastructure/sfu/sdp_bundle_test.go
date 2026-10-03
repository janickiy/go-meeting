package sfu

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/janickiy/go-recorder/internal/domain/media"
)

// firefoxBundleOffer задаёт минимальную структуру предложения Firefox без
// сетевых адресов, ICE-паролей и браузерных идентификаторов пользователя.
const firefoxBundleOffer = "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n" +
	"a=group:BUNDLE 0 1\r\n" +
	"m=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=mid:0\r\na=sendrecv\r\na=rtpmap:111 opus/48000/2\r\na=ssrc:100 cname:microphone\r\n" +
	"m=video 0 UDP/TLS/RTP/SAVPF 96\r\na=mid:1\r\na=bundle-only\r\na=sendrecv\r\na=rtpmap:96 VP8/90000\r\na=ssrc:200 cname:camera\r\n"

// TestFirefoxBundleOnlySources проверяет камеру и микрофон Firefox, приёмные
// слоты и отключённые дорожки. Проверки кодеков, типов источников и лимитов
// должны продолжать работать и для секций с общим транспортом.
//
// @args
//   - t: контекст проверки и сообщения об ошибках.
func TestFirefoxBundleOnlySources(t *testing.T) {
	publications := []media.Publication{{MID: "0", Source: media.SourceMicrophone}, {MID: "1", Source: media.SourceCamera}}
	want := map[string]media.Source{"0": media.SourceMicrophone, "1": media.SourceCamera}
	got, err := validateSourceOffer(firefoxBundleOffer, 4, publications, 4, 2, 2)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("источники Firefox: %v, %v", got, err)
	}
	legacy, err := validateOffer(firefoxBundleOffer, 4)
	if err != nil || !legacy["audio"] || !legacy["video"] {
		t.Fatalf("прежний контракт предложения: %v, %v", legacy, err)
	}
	receiveSlot := strings.Replace(firefoxBundleOffer, "BUNDLE 0 1", "BUNDLE 0 1 2", 1) +
		"m=video 0 UDP/TLS/RTP/SAVPF 96\r\na=mid:2\r\na=bundle-only\r\na=recvonly\r\na=rtpmap:96 VP8/90000\r\n"
	if got, err = validateSourceOffer(receiveSlot, 4, publications, 2, 1, 1); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("приёмный слот посчитан публикацией: %v, %v", got, err)
	}
	disabled := strings.Replace(firefoxBundleOffer, "a=bundle-only\r\n", "", 1)
	if got, err = validateSourceOffer(disabled, 4, publications[:1], 4, 2, 2); err != nil || len(got) != 1 || got["0"] != media.SourceMicrophone {
		t.Fatalf("отключённое видео ошибочно активно: %v, %v", got, err)
	}
	for _, tc := range []struct {
		name string
		raw  string
		pubs []media.Publication
		max  int
		want error
	}{
		{"отключённая камера заявлена активной", disabled, publications, 4, media.ErrInvalid},
		{"неподдерживаемый кодек", strings.Replace(firefoxBundleOffer, "VP8/90000", "H264/90000", 1), publications, 4, media.ErrInvalid},
		{"необъявленная камера", firefoxBundleOffer, publications[:1], 4, media.ErrInvalid},
		{"источник не соответствует виду секции", firefoxBundleOffer, []media.Publication{{MID: "0", Source: media.SourceCamera}, {MID: "1", Source: media.SourceMicrophone}}, 4, media.ErrInvalid},
		{"лимит публикаций", firefoxBundleOffer, publications, 1, media.ErrLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validateSourceOffer(tc.raw, 4, tc.pubs, tc.max, 2, 2); !errors.Is(err, tc.want) {
				t.Fatalf("ожидалась ошибка %v, получена %v", tc.want, err)
			}
		})
	}
}

// TestBundleOnlyRejectsMalformedGroups не даёт превратить отключённую секцию
// в активную подстановкой одного атрибута или неоднозначными MID и группами.
// Один и тот же защитный разбор применяется к предложению и к ответу.
//
// @args
//   - t: контекст проверки и сообщения об ошибках.
func TestBundleOnlyRejectsMalformedGroups(t *testing.T) {
	for _, tc := range []struct{ name, raw string }{
		{"нет BUNDLE", strings.Replace(firefoxBundleOffer, "a=group:BUNDLE 0 1\r\n", "", 1)},
		{"камера вне группы", strings.Replace(firefoxBundleOffer, "BUNDLE 0 1", "BUNDLE 0", 1)},
		{"неизвестный MID", strings.Replace(firefoxBundleOffer, "BUNDLE 0 1", "BUNDLE 0 1 9", 1)},
		{"пустая группа", strings.Replace(firefoxBundleOffer, "BUNDLE 0 1", "BUNDLE", 1)},
		{"транспорт отключён", strings.Replace(firefoxBundleOffer, "m=audio 9", "m=audio 0", 1)},
		{"bundle-only выбран транспортом", strings.Replace(firefoxBundleOffer, "BUNDLE 0 1", "BUNDLE 1 0", 1)},
		{"дублируется MID секции", strings.Replace(firefoxBundleOffer, "a=mid:1", "a=mid:0", 1)},
		{"два MID в секции", strings.Replace(firefoxBundleOffer, "a=mid:1", "a=mid:1\r\na=mid:2", 1)},
		{"повторный MID в группе", strings.Replace(firefoxBundleOffer, "BUNDLE 0 1", "BUNDLE 0 1 1", 1)},
		{"секция в двух группах", strings.Replace(firefoxBundleOffer, "a=group:BUNDLE 0 1", "a=group:BUNDLE 0 1\r\na=group:BUNDLE 0 1", 1)},
		{"атрибут без MID", strings.Replace(firefoxBundleOffer, "a=mid:1\r\n", "", 1)},
		{"атрибут со значением", strings.Replace(firefoxBundleOffer, "a=bundle-only", "a=bundle-only:true", 1)},
		{"атрибут на уровне сессии", strings.Replace(firefoxBundleOffer, "a=group:BUNDLE", "a=bundle-only\r\na=group:BUNDLE", 1)},
		{"атрибут с ненулевым портом", strings.Replace(firefoxBundleOffer, "m=video 0", "m=video 9", 1)},
		{"повторный атрибут", strings.Replace(firefoxBundleOffer, "a=bundle-only", "a=bundle-only\r\na=bundle-only", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validateSourceOffer(tc.raw, 4, nil, 4, 2, 2); !errors.Is(err, media.ErrInvalid) {
				t.Fatalf("неверное предложение принято: %v", err)
			}
			if _, err := answeredSSRCs(tc.raw); !errors.Is(err, media.ErrNegotiation) {
				t.Fatalf("неверный ответ принят: %v", err)
			}
		})
	}
}

// TestAnsweredSSRCsIncludesBundleOnly проверяет, что общий транспорт не
// блокирует разрешение SSRC после Ready, но отключённые, приёмные и ремонтные
// SSRC по-прежнему не открывают передачу медиа подписчику.
//
// @args
//   - t: контекст проверки и сообщения об ошибках.
func TestAnsweredSSRCsIncludesBundleOnly(t *testing.T) {
	raw := firefoxBundleOffer + "a=ssrc:201 cname:camera\r\na=ssrc-group:FID 200 201\r\n"
	got, err := answeredSSRCs(raw)
	if err != nil || !reflect.DeepEqual(got, map[uint32]bool{100: true, 200: true}) {
		t.Fatalf("основные SSRC общего транспорта: %v, %v", got, err)
	}
	for _, raw := range []string{
		strings.Replace(firefoxBundleOffer, "a=bundle-only\r\n", "", 1),
		strings.Replace(firefoxBundleOffer, "a=bundle-only\r\na=sendrecv", "a=bundle-only\r\na=recvonly", 1),
		strings.Replace(firefoxBundleOffer, "a=bundle-only\r\na=sendrecv", "a=bundle-only\r\na=inactive", 1),
	} {
		got, err = answeredSSRCs(raw)
		if err != nil || !reflect.DeepEqual(got, map[uint32]bool{100: true}) {
			t.Fatalf("непередающая камера попала в согласованные SSRC: %v, %v", got, err)
		}
	}
}

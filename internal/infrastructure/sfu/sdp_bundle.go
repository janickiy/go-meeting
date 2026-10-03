package sfu

import (
	"strings"

	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/pion/sdp/v3"
)

// bundleOnlySections отличает активные BUNDLE-секции с нулевым портом от
// отключённых дорожек. Firefox использует port 0 + bundle-only для камеры в
// первоначальном предложении: её транспорт задаётся первой секцией BUNDLE.
// Проверка не разрешает дорожку без существующего транспорта, повторные MID,
// неоднозначное членство в группах или произвольный атрибут bundle-only.
//
// @args
//   - description: разобранное SDP-предложение либо SDP-ответ.
//
// @return
//   - множество секций, у которых нулевой порт допустим благодаря BUNDLE;
//   - media.ErrInvalid, если структура BUNDLE некорректна.
func bundleOnlySections(description *sdp.SessionDescription) (map[*sdp.MediaDescription]bool, error) {
	sections := map[string]*sdp.MediaDescription{}
	only := map[*sdp.MediaDescription]bool{}
	for _, attr := range description.Attributes {
		if attr.Key == "bundle-only" {
			return nil, media.ErrInvalid // Этот атрибут разрешён только на уровне m-секции.
		}
	}
	for _, section := range description.MediaDescriptions {
		mid := ""
		midCount, onlyCount := 0, 0
		for _, attr := range section.Attributes {
			switch attr.Key {
			case "mid":
				midCount++
				mid = attr.Value
				if mid == "" || len(mid) > 64 || strings.ContainsAny(mid, "\r\n\x00 \t") {
					return nil, media.ErrInvalid
				}
			case "bundle-only":
				onlyCount++
				if attr.Value != "" || section.MediaName.Port.Value != 0 {
					return nil, media.ErrInvalid
				}
			}
		}
		if midCount > 1 || onlyCount > 1 || (onlyCount == 1 && midCount != 1) {
			return nil, media.ErrInvalid
		}
		if mid != "" {
			if sections[mid] != nil {
				return nil, media.ErrInvalid
			}
			sections[mid] = section
		}
		if onlyCount == 1 {
			only[section] = true
		}
	}
	grouped := map[*sdp.MediaDescription]bool{}
	for _, attr := range description.Attributes {
		if attr.Key != "group" {
			continue
		}
		fields := strings.Fields(attr.Value)
		if len(fields) == 0 || fields[0] != "BUNDLE" {
			continue
		}
		if len(fields) < 2 {
			return nil, media.ErrInvalid
		}
		anchor := sections[fields[1]]
		if anchor == nil || anchor.MediaName.Port.Value == 0 || only[anchor] {
			return nil, media.ErrInvalid
		}
		for _, mid := range fields[1:] {
			section := sections[mid]
			if section == nil || grouped[section] {
				return nil, media.ErrInvalid
			}
			grouped[section] = true
		}
	}
	for section := range only {
		if !grouped[section] {
			return nil, media.ErrInvalid
		}
	}
	return only, nil
}

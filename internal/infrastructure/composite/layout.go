// Package composite implements recording-only capture and composition. It is
// deliberately independent of the SFU forwarding loop.
package composite

import (
	"math"
	"sort"

	"github.com/janickiy/go-recorder/internal/domain/media"
)

// Tile задаёт прямоугольную область одного источника в итоговом кадре.
// @params
//   - TrackID: идентификатор связанного ресурса, заданного параметром TrackID.
//   - X: значение X типа int, используемое согласно назначению этой операции.
//   - Y: значение Y типа int, используемое согласно назначению этой операции.
//   - Width: ширина видеокадра или области в пикселях.
//   - Height: высота видеокадра или области в пикселях.
type Tile struct {
	TrackID string `json:"trackId"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
}

// Layout задаёт размер итогового кадра и расположение плиток источников.
// @params
//   - Name: имя поля, компонента или ресурса, используемое в операции.
//   - Width: ширина видеокадра или области в пикселях.
//   - Height: высота видеокадра или области в пикселях.
//   - Tiles: набор значений Tiles для последовательной или пакетной обработки.
type Layout struct {
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Tiles  []Tile `json:"tiles"`
}

// GridLayout рассчитывает стабильную сетку записи по идентификаторам участников и источников, выделяя экран отдельно от камер.
//
// @args
//   - tracks ([]media.Track): набор дорожек, входящих в операцию.
//   - width (int): ширина видеокадра или области в пикселях.
//   - height (int): высота видеокадра или области в пикселях.
//
// @return:
//   - результат 1 (Layout): значение, подготовленное операцией для вызывающей стороны.
func GridLayout(tracks []media.Track, width, height int) Layout {
	if width < 320 {
		width = 1280
	}
	if height < 240 {
		height = 720
	}
	width -= width % 2
	height -= height % 2
	result := Layout{Name: "grid", Width: width, Height: height, Tiles: []Tile{}}
	var cameras, screens []media.Track
	for _, track := range tracks {
		if track.Kind != media.KindVideo {
			continue
		}
		if track.Source == media.SourceVideoScreen {
			screens = append(screens, track)
		} else {
			cameras = append(cameras, track)
		}
	}
	// Вложенный обработчик выполняет выделенный шаг обработки в сборке и проверке аудио- и видеозаписи, используя состояние окружающей функции.
	//
	// @args
	//   - items ([]media.Track): элементы страницы или порции пакетной обработки.
	less := func(items []media.Track) {
		sort.Slice(items, /* Вложенный обработчик выполняет выделенный шаг обработки в сборке и проверке аудио- и видеозаписи, используя состояние окружающей функции.

			@args
			  - i (int): значение i типа int, используемое согласно назначению этой операции.
			  - j (int): значение j типа int, используемое согласно назначению этой операции.

			@return:
			  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(i, j int) bool {
				if items[i].ParticipantID == items[j].ParticipantID {
					return items[i].ID < items[j].ID
				}
				return items[i].ParticipantID < items[j].ParticipantID
			})
	}
	less(cameras)
	less(screens)
	if len(screens) > 0 {
		result.Name = "screen"
		mainWidth := width
		if len(cameras) > 0 {
			mainWidth = (width * 3 / 4) &^ 1
		}
		result.Tiles = append(result.Tiles, Tile{TrackID: screens[0].ID, Width: mainWidth, Height: height})
		result.Tiles = append(result.Tiles, gridTiles(cameras, mainWidth, 0, width-mainWidth, height)...)
		return result
	}
	result.Tiles = gridTiles(cameras, 0, 0, width, height)
	return result
}

// gridTiles рассчитывает прямоугольники плиток внутри области сетки.
//
// @args
//   - tracks ([]media.Track): набор дорожек, входящих в операцию.
//   - x (int): значение x типа int, используемое согласно назначению этой операции.
//   - y (int): значение y типа int, используемое согласно назначению этой операции.
//   - width (int): ширина видеокадра или области в пикселях.
//   - height (int): высота видеокадра или области в пикселях.
//
// @return:
//   - результат 1 ([]Tile): собранные элементы результата; состав ограничивается параметрами операции.
func gridTiles(tracks []media.Track, x, y, width, height int) []Tile {
	result := make([]Tile, 0, len(tracks))
	if len(tracks) == 0 {
		return result
	}
	columns := int(math.Ceil(math.Sqrt(float64(len(tracks)))))
	if width < height {
		columns = int(math.Ceil(math.Sqrt(float64(len(tracks)) * float64(width) / float64(height))))
	}
	if columns < 1 {
		columns = 1
	}
	rows := (len(tracks) + columns - 1) / columns
	tileWidth, tileHeight := (width/columns)&^1, (height/rows)&^1
	for i, track := range tracks {
		result = append(result, Tile{TrackID: track.ID, X: x + (i%columns)*tileWidth, Y: y + (i/columns)*tileHeight, Width: tileWidth, Height: tileHeight})
	}
	return result
}

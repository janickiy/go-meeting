#!/usr/bin/env python3
from __future__ import annotations

import argparse
import math
import re
from pathlib import Path

from PIL import Image, ImageDraw


# page_number извлекает номер страницы из имени изображения для правильной сортировки листов.
#
# @parameters:
#   - path (Path): путь изображения страницы.
#
# @return: int — подготовленное значение согласно назначению функции.
def page_number(path: Path) -> int:
    match = re.search(r"(\d+)$", path.stem)
    return int(match.group(1)) if match else 0


# main читает параметры командной строки и собирает листы миниатюр страниц.
#
# входные пути и настройки читаются из командной строки.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("input_dir", type=Path)
    parser.add_argument("output_dir", type=Path)
    parser.add_argument("--columns", type=int, default=4)
    parser.add_argument("--rows", type=int, default=4)
    parser.add_argument("--thumb-width", type=int, default=300)
    args = parser.parse_args()

    pages = sorted(args.input_dir.glob("page-*.png"), key=page_number)
    args.output_dir.mkdir(parents=True, exist_ok=True)
    per_sheet = args.columns * args.rows

    with Image.open(pages[0]) as first:
        ratio = first.height / first.width
    thumb_height = round(args.thumb_width * ratio)
    label_height = 24

    for sheet_index in range(math.ceil(len(pages) / per_sheet)):
        subset = pages[sheet_index * per_sheet : (sheet_index + 1) * per_sheet]
        canvas = Image.new(
            "RGB",
            (
                args.columns * args.thumb_width,
                args.rows * (thumb_height + label_height),
            ),
            "white",
        )
        draw = ImageDraw.Draw(canvas)
        for index, page in enumerate(subset):
            row, column = divmod(index, args.columns)
            x = column * args.thumb_width
            y = row * (thumb_height + label_height)
            with Image.open(page) as image:
                image = image.convert("RGB")
                image.thumbnail((args.thumb_width, thumb_height))
                canvas.paste(image, (x, y))
            draw.rectangle(
                [x, y, x + args.thumb_width - 1, y + thumb_height - 1],
                outline="#CBD5E1",
                width=1,
            )
            draw.text((x + 8, y + thumb_height + 4), f"Page {page_number(page)}", fill="#334155")
        output = args.output_dir / f"contact-{sheet_index + 1:02d}.jpg"
        canvas.save(output, quality=88, optimize=True)
        print(output)


if __name__ == "__main__":
    main()

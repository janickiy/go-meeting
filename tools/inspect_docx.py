#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
from collections import Counter
from pathlib import Path

from docx import Document
from docx.oxml.ns import qn


# has_section_break проверяет наличие разрыва раздела в XML-свойствах абзаца Word.
#
# @parameters:
#   - paragraph (объект соответствующего API): объект абзаца python-docx.
#
# @return: bool — подготовленное значение согласно назначению функции.
def has_section_break(paragraph) -> bool:
    ppr = paragraph._p.pPr
    return ppr is not None and ppr.find(qn("w:sectPr")) is not None


# main читает параметры командной строки и выводит структуру и свойства DOCX в JSON.
#
# входные пути и настройки читаются из командной строки.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("input")
    parser.add_argument("--sample-limit", type=int, default=60)
    args = parser.parse_args()

    doc = Document(args.input)
    paragraphs = doc.paragraphs

    fonts = Counter()
    sizes = Counter()
    for paragraph in paragraphs:
        for run in paragraph.runs:
            if run.font.name:
                fonts[run.font.name] += len(run.text)
            if run.font.size:
                sizes[round(run.font.size.pt, 2)] += len(run.text)

    code_groups: list[list[int]] = []
    current: list[int] = []
    for index, paragraph in enumerate(paragraphs):
        if paragraph.style and paragraph.style.name == "Preformatted Text":
            current.append(index)
        elif current:
            code_groups.append(current)
            current = []
    if current:
        code_groups.append(current)

    samples = []
    for group in code_groups[: args.sample_limit]:
        text = "\n".join(paragraphs[i].text for i in group)
        samples.append(
            {
                "start": group[0],
                "end": group[-1],
                "lines": len(group),
                "chars": len(text),
                "text": text[:1200],
            }
        )

    result = {
        "path": str(Path(args.input)),
        "paragraphs": len(paragraphs),
        "tables": len(doc.tables),
        "sections": len(doc.sections),
        "section_break_paragraphs": sum(has_section_break(p) for p in paragraphs),
        "style_counts": Counter(
            p.style.name if p.style is not None else "<none>" for p in paragraphs
        ),
        "run_fonts_by_chars": fonts,
        "run_sizes_by_chars": sizes,
        "code_groups": len(code_groups),
        "code_group_lines": Counter(len(g) for g in code_groups),
        "samples": samples,
    }
    print(json.dumps(result, ensure_ascii=False, indent=2, default=dict))


if __name__ == "__main__":
    main()

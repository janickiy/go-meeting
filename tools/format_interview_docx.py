#!/usr/bin/env python3
from __future__ import annotations

import argparse
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable

from docx import Document
from docx.enum.section import WD_ORIENT, WD_SECTION
from docx.enum.style import WD_STYLE_TYPE
from docx.enum.table import WD_CELL_VERTICAL_ALIGNMENT
from docx.enum.text import WD_ALIGN_PARAGRAPH, WD_BREAK, WD_LINE_SPACING
from docx.oxml import OxmlElement
from docx.oxml.ns import qn
from docx.shared import Cm, Pt, RGBColor


BODY_FONT = "Arial"
CODE_FONT = "Menlo"

NAVY = "163B65"
GO_BLUE = "00A6C8"
GO_DARK = "087E9A"
TEXT = "263442"
MUTED = "687784"
LIGHT = "F3F7FA"
LIGHT_ALT = "F7FAFC"
BORDER = "CFDCE6"
WHITE = "FFFFFF"

SYNTAX = {
    "plain": "243447",
    "keyword": "7C3AED",
    "builtin": "0069A8",
    "function": "145DA0",
    "string": "087F5B",
    "number": "A45100",
    "comment": "6B7280",
    "operator": "40556A",
}

LANGUAGE_LABELS = {"go", "golang", "php", "bash", "sql", "shell"}

GO_KEYWORDS = {
    "break",
    "case",
    "chan",
    "const",
    "continue",
    "default",
    "defer",
    "else",
    "fallthrough",
    "for",
    "func",
    "go",
    "goto",
    "if",
    "import",
    "interface",
    "map",
    "package",
    "range",
    "return",
    "select",
    "struct",
    "switch",
    "type",
    "var",
}

GO_BUILTINS = {
    "any",
    "append",
    "bool",
    "byte",
    "cap",
    "clear",
    "close",
    "comparable",
    "complex",
    "complex64",
    "complex128",
    "copy",
    "delete",
    "error",
    "false",
    "float32",
    "float64",
    "imag",
    "int",
    "int8",
    "int16",
    "int32",
    "int64",
    "iota",
    "len",
    "make",
    "max",
    "min",
    "new",
    "nil",
    "panic",
    "print",
    "println",
    "real",
    "recover",
    "rune",
    "string",
    "true",
    "uint",
    "uint8",
    "uint16",
    "uint32",
    "uint64",
    "uintptr",
}

SQL_KEYWORDS = {
    "alter",
    "and",
    "as",
    "begin",
    "by",
    "case",
    "commit",
    "create",
    "delete",
    "distinct",
    "drop",
    "else",
    "end",
    "from",
    "group",
    "having",
    "insert",
    "into",
    "join",
    "left",
    "limit",
    "not",
    "null",
    "on",
    "or",
    "order",
    "returning",
    "right",
    "rollback",
    "select",
    "set",
    "table",
    "then",
    "transaction",
    "union",
    "update",
    "values",
    "when",
    "where",
}

MANUAL_CODE_RANGES = [
    (61, 86),
    (92, 96),
    (109, 121),
    (131, 157),
    (181, 201),
    (216, 247),
    (251, 255),
    (258, 260),
    (352, 357),
    (360, 365),
    (368, 376),
    (413, 422),
    (424, 431),
    (446, 472),
    (495, 548),
    (4731, 4738),
]

MANUAL_REPLACEMENTS = {
    92: "type User struct {\n    Name string",
    95: "func (u *User) Rename(name string) {\n    u.Name = name",
    258: "result, err := service.Execute()\nif err != nil {",
    259: '    return fmt.Errorf("execute service: %w", err)',
    4731: "semaphore := make(chan struct{}, 5)",
    4733: "for _, task := range tasks {\n    semaphore <- struct{}{}",
    4735: "    go func(task Task) {",
    4736: "        defer func() { <-semaphore }()\n        process(task)",
    4737: "    }(task)",
}

CONTINUOUS_SECTION_BREAKS = {213, 320, 600, 4209, 4395, 4711}


# LexerState сохраняет режим лексического разбора между строками многострочного кода.
#
# @params:
#   - mode: режим многострочной строки или комментария.
@dataclass
class LexerState:
    mode: str | None = None


# CodeGroup описывает границы блока кода, язык и абзац подписи внутри документа.
#
# @params:
#   - start: начальный индекс абзаца блока.
#   - end: конечный индекс абзаца блока.
#   - language: язык подсветки кода.
#   - label_index: позиция подписи языка или None.
@dataclass
class CodeGroup:
    start: int
    end: int
    language: str
    label_index: int | None = None


# set_font назначает шрифт, размер и цвет фрагменту текста, включая XML-параметры всех наборов символов.
#
# @args
#   - run (объект соответствующего API): фрагмент текста python-docx, свойства которого изменяются.
#   - name (str): имя шрифта.
#   - size (float): размер шрифта в пунктах.
#   - color (str | None): RGB-цвет без символа #; None сохраняет прежнее значение.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def set_font(run, name: str, size: float, color: str | None = None) -> None:
    run.font.name = name
    run.font.size = Pt(size)
    if color:
        run.font.color.rgb = RGBColor.from_string(color)
    rpr = run._element.get_or_add_rPr()
    rfonts = rpr.get_or_add_rFonts()
    for attr in ("ascii", "hAnsi", "eastAsia", "cs"):
        rfonts.set(qn(f"w:{attr}"), name)


# remove_children удаляет дочерние XML-элементы указанных имён перед назначением новых свойств.
#
# @args
#   - element (объект соответствующего API): родительский XML-элемент документа.
#   - tags (Iterable[str]): локальные имена XML-полей Word для удаления.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def remove_children(element, tags: Iterable[str]) -> None:
    for tag in tags:
        for child in list(element.findall(qn(tag))):
            element.remove(child)


# set_paragraph_shading назначает или удаляет фоновую заливку абзаца без накопления повторных XML-свойств.
#
# @args
#   - paragraph (объект соответствующего API): объект абзаца python-docx.
#   - fill (str | None): RGB-цвет заливки; None удаляет заливку там, где это разрешает сигнатура.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def set_paragraph_shading(paragraph, fill: str | None) -> None:
    ppr = paragraph._p.get_or_add_pPr()
    remove_children(ppr, ["w:shd"])
    if fill:
        shd = OxmlElement("w:shd")
        shd.set(qn("w:val"), "clear")
        shd.set(qn("w:color"), "auto")
        shd.set(qn("w:fill"), fill)
        ppr.append(shd)


# set_run_shading назначает или удаляет фоновую заливку отдельного фрагмента текста.
#
# @args
#   - run (объект соответствующего API): фрагмент текста python-docx, свойства которого изменяются.
#   - fill (str | None): RGB-цвет заливки; None удаляет заливку там, где это разрешает сигнатура.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def set_run_shading(run, fill: str | None) -> None:
    rpr = run._element.get_or_add_rPr()
    remove_children(rpr, ["w:shd"])
    if fill:
        shd = OxmlElement("w:shd")
        shd.set(qn("w:val"), "clear")
        shd.set(qn("w:color"), "auto")
        shd.set(qn("w:fill"), fill)
        rpr.append(shd)


# set_paragraph_borders заменяет границы абзаца заданными XML-параметрами.
#
# @args
#   - paragraph (объект соответствующего API): объект абзаца python-docx.
#   - borders (dict[str, dict[str, str]]): словарь сторон и XML-свойств каждой границы.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def set_paragraph_borders(paragraph, borders: dict[str, dict[str, str]]) -> None:
    ppr = paragraph._p.get_or_add_pPr()
    remove_children(ppr, ["w:pBdr"])
    if not borders:
        return
    pbdr = OxmlElement("w:pBdr")
    for edge in ("top", "left", "bottom", "right"):
        if edge not in borders:
            continue
        spec = borders[edge]
        node = OxmlElement(f"w:{edge}")
        node.set(qn("w:val"), spec.get("val", "single"))
        node.set(qn("w:sz"), spec.get("sz", "8"))
        node.set(qn("w:space"), spec.get("space", "0"))
        node.set(qn("w:color"), spec.get("color", BORDER))
        pbdr.append(node)
    ppr.append(pbdr)


# set_cell_shading назначает фоновую заливку ячейки таблицы.
#
# @args
#   - cell (объект соответствующего API): ячейка таблицы python-docx.
#   - fill (str): RGB-цвет заливки; None удаляет заливку там, где это разрешает сигнатура.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def set_cell_shading(cell, fill: str) -> None:
    tcpr = cell._tc.get_or_add_tcPr()
    remove_children(tcpr, ["w:shd"])
    shd = OxmlElement("w:shd")
    shd.set(qn("w:val"), "clear")
    shd.set(qn("w:fill"), fill)
    tcpr.append(shd)


# set_cell_margins назначает внутренние отступы ячейки таблицы в единицах Word.
#
# @args
#   - cell (объект соответствующего API): ячейка таблицы python-docx.
#   - top (int): верхний отступ в единицах Word.
#   - start (int): отступ начальной стороны ячейки в единицах Word либо начальный индекс блока согласно типу.
#   - bottom (int): нижний отступ в единицах Word.
#   - end (int): отступ конечной стороны ячейки в единицах Word либо конечный индекс блока согласно типу.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def set_cell_margins(cell, top: int = 90, start: int = 110, bottom: int = 90, end: int = 110) -> None:
    tcpr = cell._tc.get_or_add_tcPr()
    margins = tcpr.first_child_found_in("w:tcMar")
    if margins is None:
        margins = OxmlElement("w:tcMar")
        tcpr.append(margins)
    for edge, value in (("top", top), ("start", start), ("bottom", bottom), ("end", end)):
        node = margins.find(qn(f"w:{edge}"))
        if node is None:
            node = OxmlElement(f"w:{edge}")
            margins.append(node)
        node.set(qn("w:w"), str(value))
        node.set(qn("w:type"), "dxa")


# set_table_borders назначает единообразные границы таблицы документа.
#
# @args
#   - table (объект соответствующего API): таблица python-docx.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def set_table_borders(table) -> None:
    tblpr = table._tbl.tblPr
    remove_children(tblpr, ["w:tblBorders"])
    borders = OxmlElement("w:tblBorders")
    for edge in ("top", "left", "bottom", "right", "insideH", "insideV"):
        node = OxmlElement(f"w:{edge}")
        node.set(qn("w:val"), "single")
        node.set(qn("w:sz"), "4")
        node.set(qn("w:space"), "0")
        node.set(qn("w:color"), BORDER)
        borders.append(node)
    tblpr.append(borders)


# set_repeat_table_header помечает строку таблицы как повторяемый заголовок на следующих страницах.
#
# @args
#   - row (объект соответствующего API): строка таблицы, повторяемая как заголовок.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def set_repeat_table_header(row) -> None:
    trpr = row._tr.get_or_add_trPr()
    remove_children(trpr, ["w:tblHeader"])
    header = OxmlElement("w:tblHeader")
    header.set(qn("w:val"), "true")
    trpr.append(header)


# clear_paragraph_runs удаляет прежние фрагменты текста абзаца перед его новым оформлением.
#
# @args
#   - paragraph (объект соответствующего API): объект абзаца python-docx.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def clear_paragraph_runs(paragraph) -> None:
    for run in list(paragraph.runs):
        paragraph._p.remove(run._element)


# set_paragraph_text заменяет текст абзаца, удаляя старые фрагменты и создавая новый.
#
# @args
#   - paragraph (объект соответствующего API): объект абзаца python-docx.
#   - text (str): исходный текст для назначения или разбора.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def set_paragraph_text(paragraph, text: str) -> None:
    clear_paragraph_runs(paragraph)
    paragraph.add_run(text)


# ensure_styles создаёт или обновляет используемые документом стили текста, заголовков и кода.
#
# @args
#   - doc (объект соответствующего API): загруженный документ python-docx.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def ensure_styles(doc) -> None:
    styles = doc.styles

    normal = styles["Normal"]
    normal.font.name = BODY_FONT
    normal.font.size = Pt(10.5)
    normal.font.color.rgb = RGBColor.from_string(TEXT)
    normal.paragraph_format.space_after = Pt(5)
    normal.paragraph_format.line_spacing = 1.12

    body = styles["Body Text"]
    body.font.name = BODY_FONT
    body.font.size = Pt(10.5)
    body.font.color.rgb = RGBColor.from_string(TEXT)
    body.paragraph_format.space_after = Pt(5)
    body.paragraph_format.line_spacing = 1.12

    title = styles["Title"]
    title.font.name = BODY_FONT
    title.font.size = Pt(29)
    title.font.bold = True
    title.font.color.rgb = RGBColor.from_string(NAVY)

    heading_tokens = {
        "Heading 1": (19, NAVY, 13, 5),
        "Heading 2": (13.5, NAVY, 9, 4),
        "Heading 3": (11.5, GO_DARK, 8, 3),
        "Heading 4": (10.5, GO_DARK, 7, 3),
    }
    for name, (size, color, before, after) in heading_tokens.items():
        style = styles[name]
        style.font.name = BODY_FONT
        style.font.size = Pt(size)
        style.font.bold = True
        style.font.color.rgb = RGBColor.from_string(color)
        style.paragraph_format.space_before = Pt(before)
        style.paragraph_format.space_after = Pt(after)
        style.paragraph_format.keep_with_next = True
        style.paragraph_format.keep_together = True
        style.paragraph_format.line_spacing = 1.02

    if "Code Block" not in styles:
        code_style = styles.add_style("Code Block", WD_STYLE_TYPE.PARAGRAPH)
    else:
        code_style = styles["Code Block"]
    code_style.base_style = normal
    code_style.font.name = CODE_FONT
    code_style.font.size = Pt(8)
    code_style.font.color.rgb = RGBColor.from_string(SYNTAX["plain"])
    code_style.paragraph_format.line_spacing = 1.0
    code_style.paragraph_format.space_before = Pt(0)
    code_style.paragraph_format.space_after = Pt(0)
    code_style.paragraph_format.left_indent = Cm(0.35)
    code_style.paragraph_format.right_indent = Cm(0.1)
    code_style.paragraph_format.first_line_indent = Cm(0)

    if "Code Label" not in styles:
        label_style = styles.add_style("Code Label", WD_STYLE_TYPE.PARAGRAPH)
    else:
        label_style = styles["Code Label"]
    label_style.base_style = normal
    label_style.font.name = BODY_FONT
    label_style.font.size = Pt(7.5)
    label_style.font.bold = True
    label_style.font.color.rgb = RGBColor.from_string(WHITE)
    label_style.paragraph_format.space_before = Pt(7)
    label_style.paragraph_format.space_after = Pt(0)
    label_style.paragraph_format.left_indent = Cm(0.35)
    label_style.paragraph_format.right_indent = Cm(0.1)
    label_style.paragraph_format.keep_with_next = True


# format_question_numbering приводит нумерацию вопросов документа к выбранному оформлению.
#
# @args
#   - doc (объект соответствующего API): загруженный документ python-docx.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def format_question_numbering(doc) -> None:
    num_ids: set[str] = set()
    for paragraph in doc.paragraphs:
        if paragraph.style is None or paragraph.style.name != "Heading 2":
            continue
        ppr = paragraph._p.pPr
        if ppr is None or ppr.numPr is None or ppr.numPr.numId is None:
            continue
        num_ids.add(ppr.numPr.numId.get(qn("w:val")))

    numbering_root = doc.part.numbering_part.element
    for num_id in num_ids:
        num = numbering_root.find(
            qn("w:num") + f"[@{qn('w:numId')}='{num_id}']"
        )
        if num is None:
            continue
        abstract_id_node = num.find(qn("w:abstractNumId"))
        if abstract_id_node is None:
            continue
        abstract_id = abstract_id_node.get(qn("w:val"))
        abstract_num = numbering_root.find(
            qn("w:abstractNum")
            + f"[@{qn('w:abstractNumId')}='{abstract_id}']"
        )
        if abstract_num is None:
            continue
        level = abstract_num.find(
            qn("w:lvl") + f"[@{qn('w:ilvl')}='0']"
        )
        if level is None:
            continue

        rpr = level.find(qn("w:rPr"))
        if rpr is None:
            rpr = OxmlElement("w:rPr")
            level.append(rpr)
        for child in list(rpr):
            rpr.remove(child)

        rfonts = OxmlElement("w:rFonts")
        for attr in ("ascii", "hAnsi", "eastAsia", "cs"):
            rfonts.set(qn(f"w:{attr}"), BODY_FONT)
        rpr.append(rfonts)
        rpr.append(OxmlElement("w:b"))
        rpr.append(OxmlElement("w:bCs"))

        color = OxmlElement("w:color")
        color.set(qn("w:val"), NAVY)
        rpr.append(color)

        size = OxmlElement("w:sz")
        size.set(qn("w:val"), "27")
        rpr.append(size)

        size_cs = OxmlElement("w:szCs")
        size_cs.set(qn("w:val"), "27")
        rpr.append(size_cs)


# normalize_sections назначает геометрию и параметры разделов документа.
#
# @args
#   - doc (объект соответствующего API): загруженный документ python-docx.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def normalize_sections(doc) -> None:
    section_break_paragraphs = [
        index
        for index, paragraph in enumerate(doc.paragraphs)
        if paragraph._p.pPr is not None
        and paragraph._p.pPr.find(qn("w:sectPr")) is not None
    ]
    for section_index, section in enumerate(doc.sections):
        section.orientation = WD_ORIENT.PORTRAIT
        section.page_width = Cm(21)
        section.page_height = Cm(29.7)
        section.left_margin = Cm(1.9)
        section.right_margin = Cm(1.9)
        section.top_margin = Cm(1.8)
        section.bottom_margin = Cm(1.7)
        section.header_distance = Cm(0.7)
        section.footer_distance = Cm(0.7)
        if section_index > 0:
            preceding_break = section_break_paragraphs[section_index - 1]
            if preceding_break in CONTINUOUS_SECTION_BREAKS:
                section.start_type = WD_SECTION.CONTINUOUS


# has_num_pr проверяет XML-признак нумерованного абзаца.
#
# @args
#   - paragraph (объект соответствующего API): объект абзаца python-docx.
#
# @return: bool — подготовленное значение согласно назначению функции.
def has_num_pr(paragraph) -> bool:
    return (
        paragraph._p.pPr is not None
        and paragraph._p.pPr.find(qn("w:numPr")) is not None
    )


# is_visual_subheading распознаёт короткий визуальный подзаголовок по тексту и исходному оформлению.
#
# @args
#   - paragraph (объект соответствующего API): объект абзаца python-docx.
#
# @return: bool — подготовленное значение согласно назначению функции.
def is_visual_subheading(paragraph) -> bool:
    if paragraph.style.name not in {"Normal", "Body Text"}:
        return False
    text = paragraph.text.strip()
    if not text or len(text) > 120:
        return False
    chars = 0
    bold_chars = 0
    large_chars = 0
    for run in paragraph.runs:
        length = len(run.text.strip())
        chars += length
        if run.bold:
            bold_chars += length
        if run.font.size and run.font.size.pt >= 14:
            large_chars += length
    return chars > 0 and (large_chars / chars >= 0.5 or (bold_chars / chars >= 0.95 and len(text) <= 65))


# apply_body_paragraph_format применяет выбранный стиль и единообразные отступы обычного абзаца.
#
# @args
#   - paragraph (объект соответствующего API): объект абзаца python-docx.
#   - style_name (str): имя выбранного стиля абзаца.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def apply_body_paragraph_format(paragraph, style_name: str) -> None:
    fmt = paragraph.paragraph_format
    fmt.keep_together = False
    fmt.keep_with_next = False
    fmt.widow_control = True
    fmt.line_spacing_rule = WD_LINE_SPACING.MULTIPLE
    fmt.line_spacing = 1.12
    fmt.space_before = Pt(0)
    fmt.space_after = Pt(5)
    fmt.first_line_indent = Cm(0)
    paragraph.alignment = WD_ALIGN_PARAGRAPH.LEFT
    if not has_num_pr(paragraph):
        fmt.left_indent = Cm(0)
        fmt.right_indent = Cm(0)

    if style_name == "Title":
        fmt.space_before = Pt(0)
        fmt.space_after = Pt(15)
        fmt.line_spacing = 1.0
        fmt.keep_with_next = True
        paragraph.alignment = WD_ALIGN_PARAGRAPH.CENTER
    elif style_name == "Heading 1":
        fmt.space_before = Pt(13)
        fmt.space_after = Pt(5)
        fmt.line_spacing = 1.0
        fmt.keep_with_next = True
        fmt.keep_together = True
    elif style_name == "Heading 2":
        fmt.space_before = Pt(9)
        fmt.space_after = Pt(4)
        fmt.line_spacing = 1.03
        fmt.keep_with_next = True
        fmt.keep_together = True
    elif style_name == "Heading 3":
        fmt.space_before = Pt(8)
        fmt.space_after = Pt(3)
        fmt.line_spacing = 1.03
        fmt.keep_with_next = True
        fmt.keep_together = True
    elif style_name == "Heading 4":
        fmt.space_before = Pt(7)
        fmt.space_after = Pt(3)
        fmt.line_spacing = 1.03
        fmt.keep_with_next = True
        fmt.keep_together = True
    elif style_name == "Horizontal Line":
        fmt.space_before = Pt(7)
        fmt.space_after = Pt(7)
        fmt.line_spacing = 1.0


# format_regular_paragraph оформляет обычный абзац с учётом заголовков, списков и его позиции.
#
# @args
#   - paragraph (объект соответствующего API): объект абзаца python-docx.
#   - paragraph_index (int | None): исходная позиция абзаца для правил оформления; None не задаёт позицию.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def format_regular_paragraph(paragraph, paragraph_index: int | None = None) -> None:
    style_name = paragraph.style.name if paragraph.style is not None else "Normal"
    if style_name == "List Paragraph" and paragraph.text.strip().endswith("?"):
        paragraph.style = "Heading 2"
        style_name = "Heading 2"
    elif is_visual_subheading(paragraph):
        paragraph.style = "Heading 3"
        style_name = "Heading 3"

    apply_body_paragraph_format(paragraph, style_name)

    if paragraph_index == 1:
        paragraph.alignment = WD_ALIGN_PARAGRAPH.CENTER
        paragraph.paragraph_format.space_after = Pt(34)
    elif paragraph_index == 5:
        paragraph.alignment = WD_ALIGN_PARAGRAPH.CENTER
        paragraph.paragraph_format.space_after = Pt(0)

    role = {
        "Title": (29, NAVY, True),
        "Heading 1": (19, NAVY, True),
        "Heading 2": (13.5, NAVY, True),
        "Heading 3": (11.5, GO_DARK, True),
        "Heading 4": (10.5, GO_DARK, True),
    }.get(style_name, (10.5, TEXT, None))

    for run in paragraph.runs:
        original_font = (run.font.name or "").lower()
        original_size = run.font.size.pt if run.font.size else None
        is_inline_code = "monospace" in original_font
        if paragraph_index == 1:
            set_font(run, BODY_FONT, 11.5, MUTED)
        elif paragraph_index == 5:
            set_font(run, BODY_FONT, 10.5, TEXT)
        elif is_inline_code and style_name not in {"Title", "Heading 1", "Heading 2", "Heading 3", "Heading 4"}:
            set_font(run, CODE_FONT, 9.0, NAVY)
            set_run_shading(run, "EDF3F7")
        else:
            set_font(run, BODY_FONT, role[0], role[1])
            if role[2] is True:
                run.bold = True
            if original_size and original_size <= 8 and style_name in {"Normal", "Body Text"}:
                set_font(run, BODY_FONT, 10.5, TEXT)


# collect_preformatted_groups находит последовательные абзацы исходного стиля предварительно форматированного текста.
#
# @args
#   - paragraphs (объект соответствующего API): абзацы документа в исходном порядке.
#
# @return: list[CodeGroup] — подготовленное значение согласно назначению функции.
def collect_preformatted_groups(paragraphs) -> list[CodeGroup]:
    groups: list[CodeGroup] = []
    start: int | None = None
    for index, paragraph in enumerate(paragraphs):
        is_code = paragraph.style is not None and paragraph.style.name == "Preformatted Text"
        if is_code and start is None:
            start = index
        elif not is_code and start is not None:
            groups.append(make_preformatted_group(paragraphs, start, index - 1))
            start = None
    if start is not None:
        groups.append(make_preformatted_group(paragraphs, start, len(paragraphs) - 1))
    return groups


# make_preformatted_group определяет язык и границы одного найденного блока кода.
#
# @args
#   - paragraphs (объект соответствующего API): абзацы документа в исходном порядке.
#   - start (int): отступ начальной стороны ячейки в единицах Word либо начальный индекс блока согласно типу.
#   - end (int): отступ конечной стороны ячейки в единицах Word либо конечный индекс блока согласно типу.
#
# @return: CodeGroup — подготовленное значение согласно назначению функции.
def make_preformatted_group(paragraphs, start: int, end: int) -> CodeGroup:
    label_index = start - 1 if start > 0 else None
    language = "go"
    if label_index is not None:
        candidate = paragraphs[label_index].text.strip().lower()
        if candidate in LANGUAGE_LABELS:
            language = "bash" if candidate == "shell" else candidate
        else:
            label_index = None
    return CodeGroup(start, end, language, label_index)


# collect_code_groups возвращает блоки кода, подготовленные для подсветки и отдельной верстки.
#
# @args
#   - paragraphs (объект соответствующего API): абзацы документа в исходном порядке.
#
# @return: list[CodeGroup] — подготовленное значение согласно назначению функции.
def collect_code_groups(paragraphs) -> list[CodeGroup]:
    groups = collect_preformatted_groups(paragraphs)
    groups.extend(CodeGroup(start, end, "go") for start, end in MANUAL_CODE_RANGES)
    groups.sort(key=lambda item: item.start)
    return groups


# tokenize_code разбирает текст кода на фрагменты подсветки с сохранением состояния многострочных конструкций.
#
# @args
#   - text (str): исходный текст для назначения или разбора.
#   - language (str): определённый язык синтаксиса блока.
#   - state (LexerState): состояние разбора, сохраняющее многострочные строки и комментарии.
#
# @return: list[tuple[str, str]] — подготовленное значение согласно назначению функции.
def tokenize_code(text: str, language: str, state: LexerState) -> list[tuple[str, str]]:
    result: list[tuple[str, str]] = []
    pieces = text.replace("\t", "    ").splitlines(keepends=True)
    if not pieces:
        pieces = [""]

    for physical_line in pieces:
        result.extend(tokenize_physical_line(physical_line, language, state))
    return result


# tokenize_physical_line разбирает одну физическую строку кода и обновляет состояние строк и комментариев.
#
# @args
#   - line (str): одна физическая строка исходного кода.
#   - language (str): определённый язык синтаксиса блока.
#   - state (LexerState): состояние разбора, сохраняющее многострочные строки и комментарии.
#
# @return: list[tuple[str, str]] — подготовленное значение согласно назначению функции.
def tokenize_physical_line(line: str, language: str, state: LexerState) -> list[tuple[str, str]]:
    result: list[tuple[str, str]] = []
    i = 0
    length = len(line)
    sql_mode = language == "sql"
    shell_mode = language == "bash"
    php_mode = language == "php"

    while i < length:
        if state.mode == "block_comment":
            end = line.find("*/", i)
            if end < 0:
                result.append(("comment", line[i:]))
                return result
            result.append(("comment", line[i : end + 2]))
            i = end + 2
            state.mode = None
            continue

        if state.mode == "raw_string":
            end = line.find("`", i)
            if end < 0:
                result.append(("string", line[i:]))
                return result
            result.append(("string", line[i : end + 1]))
            i = end + 1
            state.mode = None
            continue

        if line.startswith("/*", i):
            end = line.find("*/", i + 2)
            if end < 0:
                result.append(("comment", line[i:]))
                state.mode = "block_comment"
                return result
            result.append(("comment", line[i : end + 2]))
            i = end + 2
            continue

        if line.startswith("//", i) or (sql_mode and line.startswith("--", i)):
            result.append(("comment", line[i:]))
            return result
        if shell_mode and line[i] == "#":
            result.append(("comment", line[i:]))
            return result

        ch = line[i]
        if ch == "`" and not sql_mode:
            end = line.find("`", i + 1)
            if end < 0:
                result.append(("string", line[i:]))
                state.mode = "raw_string"
                return result
            result.append(("string", line[i : end + 1]))
            i = end + 1
            continue

        if ch in {'"', "'"}:
            quote = ch
            j = i + 1
            escaped = False
            while j < length:
                current = line[j]
                if escaped:
                    escaped = False
                elif current == "\\":
                    escaped = True
                elif current == quote:
                    j += 1
                    break
                elif current in "\r\n":
                    break
                j += 1
            result.append(("string", line[i:j]))
            i = j
            continue

        if ch.isspace():
            j = i + 1
            while j < length and line[j].isspace():
                j += 1
            result.append(("plain", line[i:j]))
            i = j
            continue

        number = re.match(
            r"(?:0[xX][0-9a-fA-F_]+|0[bB][01_]+|0[oO][0-7_]+|(?:\d[\d_]*)(?:\.\d[\d_]*)?(?:[eE][+-]?\d[\d_]*)?i?)",
            line[i:],
        )
        if number:
            value = number.group(0)
            result.append(("number", value))
            i += len(value)
            continue

        identifier = re.match(r"[A-Za-z_][A-Za-z0-9_]*", line[i:])
        if identifier:
            value = identifier.group(0)
            lower = value.lower()
            if language in {"go", "golang"} and value in GO_KEYWORDS:
                kind = "keyword"
            elif language in {"go", "golang"} and value in GO_BUILTINS:
                kind = "builtin"
            elif sql_mode and lower in SQL_KEYWORDS:
                kind = "keyword"
            else:
                next_index = i + len(value)
                while next_index < length and line[next_index].isspace():
                    next_index += 1
                kind = "function" if next_index < length and line[next_index] == "(" else "plain"
            result.append((kind, value))
            i += len(value)
            continue

        if php_mode and ch == "$":
            variable = re.match(r"\$[A-Za-z_][A-Za-z0-9_]*", line[i:])
            if variable:
                value = variable.group(0)
                result.append(("builtin", value))
                i += len(value)
                continue

        if ch in "+-*/%=&|!<>:^~":
            j = i + 1
            while j < length and line[j] in "+-*/%=&|!<>:^~":
                j += 1
            result.append(("operator", line[i:j]))
            i = j
            continue

        result.append(("plain", ch))
        i += 1

    return result


# format_language_label оформляет подпись языка перед блоком кода.
#
# @args
#   - paragraph (объект соответствующего API): объект абзаца python-docx.
#   - language (str): определённый язык синтаксиса блока.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def format_language_label(paragraph, language: str) -> None:
    label = "GO" if language in {"go", "golang"} else language.upper()
    set_paragraph_text(paragraph, label)
    paragraph.style = "Code Label"
    paragraph.alignment = WD_ALIGN_PARAGRAPH.LEFT
    fmt = paragraph.paragraph_format
    fmt.left_indent = Cm(0.35)
    fmt.right_indent = Cm(0.1)
    fmt.first_line_indent = Cm(0)
    fmt.space_before = Pt(7)
    fmt.space_after = Pt(0)
    fmt.line_spacing = 1.0
    fmt.keep_with_next = True
    set_paragraph_shading(paragraph, NAVY)
    set_paragraph_borders(
        paragraph,
        {
            "top": {"sz": "8", "color": NAVY},
            "left": {"sz": "12", "color": GO_BLUE},
            "right": {"sz": "4", "color": NAVY},
        },
    )
    for run in paragraph.runs:
        set_font(run, BODY_FONT, 7.5, WHITE)
        run.bold = True


# format_code_group назначает блоку кода шрифт, фон, интервалы и подсветку синтаксиса.
#
# @args
#   - paragraphs (объект соответствующего API): абзацы документа в исходном порядке.
#   - group (CodeGroup): границы и язык блока, который требуется оформить.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def format_code_group(paragraphs, group: CodeGroup) -> None:
    if group.label_index is not None:
        format_language_label(paragraphs[group.label_index], group.language)

    state = LexerState()
    is_go = group.language in {"go", "golang"}
    physical_line_counts = [
        max(1, len(paragraphs[index].text.replace("\t", "    ").splitlines()))
        for index in range(group.start, group.end + 1)
    ]
    total_lines = sum(physical_line_counts)
    line_number_width = len(str(total_lines))
    line_number = 1

    for index in range(group.start, group.end + 1):
        paragraph = paragraphs[index]
        source = paragraph.text.replace("\t", "    ")
        clear_paragraph_runs(paragraph)
        paragraph.style = "Code Block"
        paragraph.alignment = WD_ALIGN_PARAGRAPH.LEFT
        fmt = paragraph.paragraph_format
        fmt.left_indent = Cm(0.35)
        fmt.right_indent = Cm(0.1)
        fmt.first_line_indent = Cm(0)
        fmt.space_before = Pt(0)
        fmt.space_after = Pt(0)
        fmt.line_spacing_rule = WD_LINE_SPACING.SINGLE
        fmt.line_spacing = 1.0
        fmt.keep_together = True
        fmt.keep_with_next = False
        fmt.widow_control = False
        set_paragraph_shading(paragraph, LIGHT)

        border_spec = {
            "left": {"sz": "12", "color": GO_BLUE},
            "right": {"sz": "4", "color": BORDER},
        }
        if index == group.start and group.label_index is None:
            border_spec["top"] = {"sz": "4", "color": BORDER}
            fmt.space_before = Pt(7)
        if index == group.end:
            border_spec["bottom"] = {"sz": "4", "color": BORDER}
            fmt.space_after = Pt(7)
        set_paragraph_borders(paragraph, border_spec)

        physical_lines = source.splitlines(keepends=True) or [""]
        added_content = False
        for physical_line in physical_lines:
            if is_go:
                number_run = paragraph.add_run(
                    f"{line_number:>{line_number_width}}  "
                )
                set_font(number_run, CODE_FONT, 7.5, "8291A0")
                set_run_shading(number_run, "E5EDF3")
                number_run.bold = False
                number_run.italic = False
                line_number += 1
                added_content = True

            tokens = tokenize_physical_line(
                physical_line, group.language, state
            )
            for kind, value in tokens:
                if not value:
                    continue
                run = paragraph.add_run(value)
                set_font(run, CODE_FONT, 8.0, SYNTAX[kind])
                if kind == "comment":
                    run.italic = True
                elif kind == "function":
                    run.bold = True
                added_content = True

        if not added_content:
            run = paragraph.add_run("")
            set_font(run, CODE_FONT, 8.0, SYNTAX["plain"])


# format_tables оформляет таблицы документа, включая заголовки, границы и отступы.
#
# @args
#   - doc (объект соответствующего API): загруженный документ python-docx.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def format_tables(doc) -> None:
    for table in doc.tables:
        table.alignment = 1
        set_table_borders(table)
        if table.rows:
            set_repeat_table_header(table.rows[0])
        for row_index, row in enumerate(table.rows):
            for cell in row.cells:
                cell.vertical_alignment = WD_CELL_VERTICAL_ALIGNMENT.CENTER
                set_cell_margins(cell)
                set_cell_shading(cell, NAVY if row_index == 0 else (LIGHT_ALT if row_index % 2 == 0 else WHITE))
                for paragraph in cell.paragraphs:
                    paragraph.alignment = WD_ALIGN_PARAGRAPH.LEFT
                    paragraph.paragraph_format.space_before = Pt(0)
                    paragraph.paragraph_format.space_after = Pt(2)
                    paragraph.paragraph_format.line_spacing = 1.05
                    for run in paragraph.runs:
                        set_font(run, BODY_FONT, 9.25, WHITE if row_index == 0 else TEXT)
                        if row_index == 0:
                            run.bold = True


# format_headers_and_footers оформляет колонтитулы разделов и служебные сведения страниц.
#
# @args
#   - doc (объект соответствующего API): загруженный документ python-docx.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def format_headers_and_footers(doc) -> None:
    seen: set[int] = set()
    for section in doc.sections:
        for container in (section.header, section.footer):
            if id(container._element) in seen:
                continue
            seen.add(id(container._element))
            for paragraph in container.paragraphs:
                paragraph.paragraph_format.space_before = Pt(0)
                paragraph.paragraph_format.space_after = Pt(0)
                for run in paragraph.runs:
                    set_font(run, BODY_FONT, 8.5, MUTED)
            for table in container.tables:
                for row in table.rows:
                    for cell in row.cells:
                        for paragraph in cell.paragraphs:
                            paragraph.paragraph_format.space_before = Pt(0)
                            paragraph.paragraph_format.space_after = Pt(0)
                            for run in paragraph.runs:
                                set_font(run, BODY_FONT, 8.5, MUTED)


# add_cover_accent добавляет визуальное оформление абзацу титульной страницы.
#
# @args
#   - paragraph (объект соответствующего API): объект абзаца python-docx.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def add_cover_accent(paragraph) -> None:
    set_paragraph_borders(
        paragraph,
        {"bottom": {"sz": "16", "space": "8", "color": GO_BLUE}},
    )


# format_document читает исходный DOCX, применяет оформление ко всем его блокам и сохраняет новый документ.
#
# @args
#   - input_path (Path): исходный DOCX-файл для чтения.
#   - output_path (Path): путь сохраняемого оформленного DOCX-файла.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def format_document(input_path: Path, output_path: Path) -> None:
    doc = Document(str(input_path))
    paragraphs = doc.paragraphs

    if len(paragraphs) != 4748:
        raise ValueError(
            f"Unexpected paragraph count: {len(paragraphs)}; expected 4748 for this source document"
        )

    for index, replacement in MANUAL_REPLACEMENTS.items():
        set_paragraph_text(paragraphs[index], replacement)

    ensure_styles(doc)
    normalize_sections(doc)
    code_groups = collect_code_groups(paragraphs)
    code_indexes = {
        index
        for group in code_groups
        for index in range(group.start, group.end + 1)
    }
    label_indexes = {
        group.label_index for group in code_groups if group.label_index is not None
    }

    for index, paragraph in enumerate(paragraphs):
        if index in code_indexes or index in label_indexes:
            continue
        format_regular_paragraph(paragraph, index)

    # Сохраняем заголовок категории «Тестирование» вместе с начинающимся разделом.
    paragraphs[4395].paragraph_format.page_break_before = True

    for group in code_groups:
        format_code_group(paragraphs, group)

    format_question_numbering(doc)
    add_cover_accent(paragraphs[1])
    format_tables(doc)
    format_headers_and_footers(doc)

    output_path.parent.mkdir(parents=True, exist_ok=True)
    doc.save(str(output_path))
    print(
        f"Saved {output_path} with {len(code_groups)} code blocks "
        f"({sum(group.end - group.start + 1 for group in code_groups)} code paragraphs)"
    )


# main читает параметры командной строки и запускает оформление DOCX.
#
# входные пути и настройки читаются из командной строки.
#
# @return: возвращаемого значения нет; изменяет переданные объекты или сохраняет результат операции.
def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("input", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    format_document(args.input, args.output)


if __name__ == "__main__":
    main()

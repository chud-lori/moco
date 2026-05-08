import sys

try:
    from docling.document_converter import DocumentConverter
except Exception as exc:
    raise SystemExit(f"IMPORT_ERROR:{exc}")


def main() -> None:
    converter = DocumentConverter()
    result = converter.convert(sys.argv[1])
    markdown = result.document.export_to_markdown()
    sys.stdout.write(markdown if isinstance(markdown, str) else "")


if __name__ == "__main__":
    main()

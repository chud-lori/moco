import sys

try:
    import pymupdf4llm
except Exception as exc:
    raise SystemExit(f"IMPORT_ERROR:{exc}")


def main() -> None:
    markdown = pymupdf4llm.to_markdown(sys.argv[1])
    sys.stdout.write(markdown if isinstance(markdown, str) else "")


if __name__ == "__main__":
    main()

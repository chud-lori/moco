import sys

try:
    from marker.converters.pdf import PdfConverter
    from marker.models import create_model_dict
    from marker.output import text_from_rendered
except Exception as exc:
    raise SystemExit(f"IMPORT_ERROR:{exc}")


def main() -> None:
    converter = PdfConverter(artifact_dict=create_model_dict())
    rendered = converter(sys.argv[1])
    text, _, _ = text_from_rendered(rendered)
    sys.stdout.write(text if isinstance(text, str) else "")


if __name__ == "__main__":
    main()

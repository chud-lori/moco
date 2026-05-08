import sys

try:
    from nougat import NougatModel  # noqa: F401
    from nougat.dataset.rasterize import rasterize_paper  # noqa: F401
except Exception as exc:
    raise SystemExit(f"IMPORT_ERROR:{exc}")

raise SystemExit("IMPORT_ERROR:nougat Python API integration is not available in this environment")

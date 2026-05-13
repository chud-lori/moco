import * as pdfjsLib from "/static/vendor/pdf.min.mjs";
pdfjsLib.GlobalWorkerOptions.workerSrc = "/static/vendor/pdf.worker.min.mjs";
window.__mocoPdfjs = pdfjsLib;

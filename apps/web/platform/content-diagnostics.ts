export async function copyDiagnosticId(value:string){await navigator.clipboard.writeText(value)}
// Saves the export response exactly as the server sent it. Re-encoding a
// parsed bundle here would rename every key, and the file is meant to be read
// next to the server's own logs. The shape is structurally DiagnosticDownload;
// it is spelled out rather than imported because this file is not a declared
// content adapter (scripts/content-boundaries.json).
export function downloadDiagnosticBundle({blob,filename}:{blob:Blob;filename:string}){const url=URL.createObjectURL(blob);const link=document.createElement("a");link.href=url;link.download=filename;link.click();setTimeout(()=>URL.revokeObjectURL(url),1000)}

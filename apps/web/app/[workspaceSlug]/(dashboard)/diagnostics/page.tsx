"use client";
import {useWorkspaceId} from "@multica/core/hooks";
import {DiagnosticsPage} from "@multica/views/content/diagnostics";
import {copyDiagnosticId,downloadDiagnosticBundle} from "../../../../platform/content-diagnostics";
export default function Page(){const wsId=useWorkspaceId();return <DiagnosticsPage key={wsId} wsId={wsId} copy={copyDiagnosticId} download={downloadDiagnosticBundle}/>}

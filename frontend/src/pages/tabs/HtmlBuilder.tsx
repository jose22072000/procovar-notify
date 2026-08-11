import type { ReactNode } from "react";
import CodeMirror from "@uiw/react-codemirror";
import { html as htmlLang } from "@codemirror/lang-html";
import { AlignLeft, Undo2 } from "lucide-react";
import { Card, ErrorText } from "../../ui";
import { Button } from "@/components/ui/button";
import { VariablesPanel } from "../../components/VariablesPanel";
import { MissingVarsNotice, PreviewPausedNotice } from "../../components/PreviewNotices";
import type { Preview, VariableSpec } from "./TemplateBuilder";

// HtmlBuilder: modo avanzado. Editor de código (HTML/CSS crudo) a la izquierda y
// vista previa en vivo (iframe en sandbox) a la derecha, con el panel de variables
// de prueba. El HTML se sanea en el servidor al previsualizar y al guardar.
export default function HtmlBuilder({
  body,
  onBodyChange,
  detected,
  variables,
  onVariablesChange,
  preview,
  previewErr,
  previewPaused,
  testValue,
  onTestChange,
  canRevert,
  onRevert,
  onFormat,
  footer,
}: {
  body: string;
  onBodyChange: (v: string) => void;
  detected: string[];
  variables: Record<string, VariableSpec>;
  onVariablesChange: (v: Record<string, VariableSpec>) => void;
  preview: Preview | null;
  previewErr: unknown;
  previewPaused: boolean;
  testValue: (name: string) => string;
  onTestChange: (name: string, val: string) => void;
  canRevert: boolean;
  onRevert: () => void;
  onFormat: () => void;
  footer?: ReactNode;
}) {
  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:items-start">
      {/* Izquierda: editor de código + objeto que se enviará */}
      <div className="space-y-4">
      <Card
        title="Editor HTML"
        actions={
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={onFormat}>
              <AlignLeft />
              Formatear
            </Button>
            {canRevert && (
              <Button variant="outline" size="sm" onClick={onRevert}>
                <Undo2 />
                Volver a visual
              </Button>
            )}
          </div>
        }
      >
        <p className="-mt-1 mb-2 text-xs text-muted-foreground">
          HTML y CSS completos (el CSS va en un «{`<style>`}»). Usa «{`{{variable}}`}» para insertar datos. Se sanea al guardar: se elimina JavaScript y contenido no permitido.
        </p>
        <div className="overflow-hidden rounded-lg border border-border">
          <CodeMirror
            value={body}
            onChange={onBodyChange}
            extensions={[htmlLang()]}
            height="560px"
            basicSetup={{ lineNumbers: true, foldGutter: true, highlightActiveLine: true }}
          />
        </div>
      </Card>

        {/* El objeto que se enviará, debajo del editor de código. */}
        {footer}
      </div>

      {/* Derecha: preview en vivo (sandbox) + variables */}
      <div className="space-y-4 lg:sticky lg:top-4">
        <Card title="Vista previa">
          <ErrorText error={previewErr} />
          <PreviewPausedNotice paused={previewPaused} />
          <MissingVarsNotice missing={preview?.missingVariables} />
          {preview ? (
            <iframe title="preview" sandbox="" className="h-[32rem] w-full rounded-lg border border-border bg-white" srcDoc={preview.html} />
          ) : (
            <p className="text-sm text-muted-foreground">Renderizando…</p>
          )}
        </Card>

        <VariablesPanel detected={detected} specs={variables} onChange={onVariablesChange} testValue={testValue} onTestChange={onTestChange} />
      </div>
    </div>
  );
}

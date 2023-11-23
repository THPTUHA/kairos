import {  WorkflowFile } from "../models/workflow";
import { ObjectEditor } from "./ObjectEditor";

export function WorkflowEditor({
    onChange,
    template
}: {
    template: WorkflowFile;
    onChange: (template: WorkflowFile) => void;
}) {
    return (
        <ObjectEditor  value={template} onChange={x => onChange({...x})} />
    );
}

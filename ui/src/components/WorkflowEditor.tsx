import { Workflow } from "../models/workflow";
import { ObjectEditor } from "./ObjectEditor";

export function WorkflowEditor({
    onChange,
    template
}: {
    template: Workflow;
    onChange: (template: Workflow) => void;
}) {
    return (
        <ObjectEditor  value={template} onChange={x => onChange({...x})} />
    );
}

import { useState } from "react";
import { UploadButton } from "../../components/UploadButton"
import { Workflow } from "../../models/workflow";
import { services } from "../../services";
import { WorkflowEditor } from "../../components/WorkflowEditor";

const WorkflowListPage = () => {
    const [workflow, setWorkflow] = useState<Workflow>();
    const [error, setError] = useState<Error>();
    function onCreate(wf: Workflow){
        console.log("workflow created")
    }
    return (
        <div>
            {
                workflow && <WorkflowEditor template={workflow} onChange={setWorkflow} />
            }
            <UploadButton onUpload={setWorkflow} onError={setError} />
            <button onClick={() => {
                    console.log({})
                    if(workflow){
                        services.workflows
                        .create(workflow)
                        .then(onCreate)
                        .catch(setError);
                    }
                }}
            className="bg-green-500"
            > Apply</button>
        </div>
    )
}

export default WorkflowListPage
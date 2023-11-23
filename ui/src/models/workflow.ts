
export const SetStatusWorkflow = 0
export interface MonitorWorkflow {
    cmd :number;
    data: any;
}
export interface Workflow {
    id: number;
    key: number;
    version: string;
    name:string;
    namesapce: string;
    created_at: string;
    raw_data: string;
    file: WorkflowFile;
    status: number;
}

export interface WorkflowFile {
    version: string;
    
}

export const NODE_PHASE = {
    PENDING: 'Pending',
    RUNNING: 'Running',
    SUCCEEDED: 'Succeeded',
    SKIPPED: 'Skipped',
    FAILED: 'Failed',
    ERROR: 'Error',
    OMITTED: 'Omitted'
};

export interface ValueFrom {
    /**
     * JQFilter expression against the resource object in resource templates
     */
    jqFilter?: string;
    /**
     * JSONPath of a resource to retrieve an output parameter value from in resource templates
     */
    jsonPath?: string;
    /**
     * Parameter reference to a step or dag task in which to retrieve an output parameter value from (e.g. '{{steps.mystep.outputs.myparam}}')
     */
    parameter?: string;
    /**
     * Path in the container to retrieve an output parameter value from in container templates
     */
    path?: string;
}


/**
 * Parameter indicate a passed string parameter to a service template with an optional default value
 */
export interface Parameter {
    /**
     * Default is the default value to use for an input parameter if a value was not supplied
     */
    default?: string;
    /**
     * Name is the parameter name
     */
    name: string;
    /**
     * Value is the literal value to use for the parameter. If specified in the context of an input parameter, the value takes precedence over any passed values
     */
    value?: string;
    /**
     * ValueFrom is the source for the output parameter's value
     */
    valueFrom?: ValueFrom;
    /**
     * Enum holds a list of string values to choose from, for the actual value of the parameter
     */
    enum?: Array<string>;
    /**
     * Description is the parameter description
     */
    description?: string;
}

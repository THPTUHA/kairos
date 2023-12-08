import { CertService } from "./certificateService";
import { ChannelService } from "./channelService";
import { ClientService } from "./clientService";
import { FunctionSerive } from "./functionService";
import { GraphService } from "./graphService";
import { RecordService } from "./recordService";
import { UserService } from "./userService";
import { WorkflowsService } from "./workflowService";

interface Services {
    workflows: typeof WorkflowsService;
    clients:  typeof ClientService;
    users: typeof UserService;
    channels: typeof ChannelService;
    certs: typeof CertService;
    functions: typeof FunctionSerive;
    graphs: typeof GraphService;
    records: typeof RecordService;
}

export const services: Services = {
    workflows: WorkflowsService,
    clients: ClientService,
    users: UserService,
    channels: ChannelService,
    certs: CertService,
    functions: FunctionSerive,
    graphs: GraphService,
    records: RecordService,
};

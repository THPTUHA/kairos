const grpc = require('grpc');
const protoLoader = require('@grpc/proto-loader');
const path = require('path')
const { HealthImplementation } = require('grpc-health-check');

const packageDefinition = protoLoader.loadSync(path.join(path.dirname(__filename), "executor.proto"), {
    keepCase: true,
    longs: String,
    enums: String,
    defaults: true,
    oneofs: true,
});

const executorProto = grpc.loadPackageDefinition(packageDefinition).proto;

const PORT = 50051;

const executorService = {
    Execute: (call, callback) => {
        const { task_id, config, status_server } = call.request;
        
        const response = {
            output: Buffer.from('?????' + call),
            error: '', 
        };

        callback(null, response);
    },
};


const statusMap = {
    'plugin': 'SERVING',
    '': 'NOT_SERVING',
};

const healthImpl = new HealthImplementation(statusMap);
healthImpl.setStatus('plugin', 'SERVING');

const server = new grpc.Server();
server.addService(executorProto.Executor.service, executorService);
healthImpl.addToServer(server)

const HOST = 'localhost';

server.bind(`${HOST}:${PORT}`, grpc.ServerCredentials.createInsecure());
console.log(`1|tcp|localhost:50051|grpc`);
server.start();

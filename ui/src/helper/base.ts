
function serverUrl(): string{
    return "http://localhost:8001/";
}

function baseUrl(): string{
    return "http://localhost:3000/";
}

export function uiUrl(uiPath: string): string {
    return baseUrl() + uiPath;
}

export function uiUrlWithParams(uiPath: string, params: string[]): string {
    if (!params.length) {
        return uiUrl(uiPath);
    }
    return baseUrl() + uiPath + '?' + params.join('&');
}

export function apiUrl(apiPath: string): string {
    return `${serverUrl()}${apiPath}`;
}

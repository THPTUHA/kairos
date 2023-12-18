
function serverUrl(): string{
    return process.env.REACT_APP_HTTPSERVER ? process.env.REACT_APP_HTTPSERVER : "";
}

function baseUrl(): string{
    return process.env.REACT_APP_BASE_URL ? process.env.REACT_APP_BASE_URL : "";
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

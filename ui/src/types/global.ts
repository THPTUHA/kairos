export {};

declare global {
  interface Window {
    __RUNTIME_CONFIG__: {
      NODE_ENV: string;
      REACT_APP_DEFAULT_FILTER: string;
      REACT_APP_HUB_HOST: string;
      REACT_APP_HUB_PORT: string;
    };
  }
}

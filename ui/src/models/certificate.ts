export interface Certificate {
    id: number;
    key: string;
    user_id: string;
    api_key: string;
    secret_key: string;
    expire_at: number;
    created_at: number;
}
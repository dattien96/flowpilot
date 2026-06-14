export interface HttpClient {
  request(input: RequestInfo | URL, init?: RequestInit): Promise<Response>;
}

export const fetchHttpClient: HttpClient = {
  request(input, init) {
    return fetch(input, init);
  },
};

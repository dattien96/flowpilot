export class RedirectError extends Error {
  constructor(public readonly url: string) {
    super(`Redirecting to ${url}`);
  }
}

export function redirect(url: string): never {
  throw new RedirectError(url);
}

export function notFound(): never {
  throw new Error("Not found");
}

export function usePathname() {
  return "";
}

export function useRouter() {
  return {
    push: () => {},
    replace: () => {},
    refresh: () => {},
  };
}

export class NextResponse extends Response {
  static json(body: any, init?: ResponseInit) {
    return Response.json(body, init);
  }
  static redirect(url: string | URL, status: number = 307) {
    return new Response(null, {
      status,
      headers: { Location: String(url) },
    });
  }
}

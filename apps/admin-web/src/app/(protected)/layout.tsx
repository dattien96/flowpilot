import { requireAdminSession } from "@/data/auth/session";
import { AppShell } from "@/presentation/components/layout/app-shell";

export default async function ProtectedLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  const session = await requireAdminSession();

  return <AppShell session={session}>{children}</AppShell>;
}

export default function SettingsPage() {
  return (
    <div className="space-y-6">
      <header>
        <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
          Settings
        </p>
        <h1 className="mt-3 text-4xl font-semibold tracking-tight">Integration boundary</h1>
      </header>
      <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
        <p className="text-muted-foreground">
          The current skeleton runs in demo mode. Supabase auth, Postgres,
          storage, realtime, and a real workflow backend will plug into the same
          domain gateway contracts later.
        </p>
      </div>
    </div>
  );
}

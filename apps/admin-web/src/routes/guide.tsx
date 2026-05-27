import { Link, createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import {
  ShieldCheck,
  ShieldAlert,
  Key,
  Cpu,
  Sparkles,
  Check,
  ChevronDown,
  ChevronUp,
  ArrowRight,
  Terminal,
  Play,
  User,
  Plus,
  RefreshCw,
  FolderOpen,
  FileText,
  MoreVertical,
  Copy,
  X
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { useAuth } from "@/features/auth/auth-provider";

export const Route = createFileRoute("/guide")({
  component: GuidePage,
});

export default function GuidePage() {
  const { session } = useAuth();
  const [activeFaq, setActiveFaq] = useState<number | null>(null);

  const toggleFaq = (index: number) => {
    setActiveFaq(activeFaq === index ? null : index);
  };

  const faqs = [
    {
      q: "How does FlowPilot guarantee that the AI won't skip steps?",
      a: "In traditional coding assistants, prompts are sent as one big instructions markdown file (like CLAUDE.md), and the AI decides what to do. In FlowPilot, the workflow is split into separate execution steps. Each step is individually loaded, evaluated, and executed by our local Go-runner. The AI physically cannot skip steps or advance without the Go-runner reporting a successful completion of the current step."
    },
    {
      q: "What are Un-bypassable Safe Gates?",
      a: "Safe Gates are approval milestones hard-coded into your pipeline definition. When a step requires approval (e.g. after planning or design), execution is completely paused and locked in the database. The AI runner halts and waits for a physical webhook trigger from the human user. There is no prompt injection or AI logic that can override this check—it is secured at the system runner level."
    },
    {
      q: "How does Context Harnessing reduce token costs?",
      a: "Rather than pasting full codebases, FlowPilot automatically indexes your previous step outputs, ticket briefs, and Figma scopes. It runs a pgvector semantic search to retrieve ONLY the directly relevant content and dependencies for the active step. It then prunes redundant files, keeping the prompt context window tight, which saves 60-90% on token fees and increases implementation accuracy."
    },
    {
      q: "How does FlowPilot handle cross-machine continuity?",
      a: "Traditional tools store LLM conversational state in local directories (like ~/.codex/). If you switch from your office PC to a laptop, the AI forgets the thread. FlowPilot syncs all step definitions, logs, and artifacts to a secure cloud database. When starting on a new machine, you simply bind your local directory path, and FlowPilot will generate a 'Bootstrap Replay' prompt from the cloud history to instantly catch the AI up to speed."
    },
    {
      q: "Can I run workflows in fully automatic mode?",
      a: "Yes. Every workflow run has a YOLO mode toggle. When YOLO is enabled, all non-critical approval gates are bypassed, allowing the AI runner to execute and compile code autonomously in a loop. FlowPilot monitors tests, compiler checks, and wrong-direction loops to automatically roll back or raise alerts if the agent gets stuck."
    }
  ];

  return (
    <div className="noise-bg min-h-screen text-foreground selection:bg-accent/30 font-sans">
      {/* 2. Header and Navigation */}
      <header className="sticky top-0 z-50 border-b border-border/80 bg-background/80 backdrop-blur-md transition-all">
        <div className="mx-auto flex max-w-7xl items-center justify-between px-6 py-4">
          <div className="flex items-center space-x-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-accent text-accent-foreground">
              <Cpu className="h-5 w-5" />
            </div>
            <div>
              <span className="font-mono text-lg font-bold tracking-tight text-accent dark:text-accent-foreground">FlowPilot</span>
              <span className="ml-1.5 rounded-full bg-accent/10 px-2 py-0.5 text-xs font-medium text-accent">MVP</span>
            </div>
          </div>
          <nav className="hidden md:flex items-center space-x-8 text-sm font-medium text-muted-foreground">
            <a href="#superpowers" className="hover:text-accent transition-colors">Core Superpowers</a>
            <a href="#how-it-works" className="hover:text-accent transition-colors">How It Works</a>
            <a href="#faq" className="hover:text-accent transition-colors">FAQ</a>
          </nav>
          <div className="flex items-center space-x-4">
            {!session && (
              <Link to="/login">
                <Button variant="secondary" className="px-5 py-2">Sign In</Button>
              </Link>
            )}
          </div>
        </div>
      </header>

      {/* 3. Hero Section (SEO Optimized Title & Subtitle) */}
      <section className="relative mx-auto max-w-7xl px-6 pt-16 pb-24 text-center lg:pt-24">
        <div className="inline-flex items-center space-x-2 rounded-full border border-border/60 bg-card/60 px-3 py-1.5 backdrop-blur-sm">
          <Sparkles className="h-4 w-4 text-warning" />
          <span className="font-mono text-xs tracking-wider uppercase text-muted-foreground">The AI-Assisted Engineering Operating Layer</span>
        </div>

        <h1 className="mt-8 font-sans text-5xl font-extrabold tracking-tight sm:text-6xl md:text-7xl lg:text-8xl">
          AI Code Workflows, <br />
          <span className="text-accent">Durable & Controlled.</span>
        </h1>

        <p className="mx-auto mt-6 max-w-3xl text-lg text-muted-foreground sm:text-xl leading-relaxed">
          Stop relying on fragile, single-session chat prompts. FlowPilot wraps your local AI provider CLIs inside structured pipelines, enforcing step rules, safe gates, and machine-portable context.
        </p>

        {/* 4. Hero CTA */}
        <div className="mt-10 flex flex-col items-center justify-center space-y-4 sm:flex-row sm:space-y-0 sm:space-x-4">
          <Link to={session ? "/dashboard" : "/login"}>
            <Button className="px-8 py-4 text-base font-bold shadow-xl hover:scale-[1.02] transition-transform">
              {session ? "Go to Dashboard" : "Get Started"}
            </Button>
          </Link>
          <a href="#how-it-works">
            <Button variant="secondary" className="px-8 py-4 text-base font-semibold">
              See the Runner in Action
            </Button>
          </a>
        </div>

        {/* 5. Social Proof */}
        <div className="mt-16 border-t border-border/40 pt-10">
          <p className="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">Architected For</p>
          <div className="mt-6 flex flex-wrap items-center justify-center gap-10 opacity-70">
            <div className="flex items-center space-x-2">
              <span className="font-bold text-lg">SOLO DEVS</span>
              <span className="text-xs text-muted-foreground">• Full Lifecycle Autonomy</span>
            </div>
            <div className="flex items-center space-x-2">
              <span className="font-bold text-lg">TECH LEADERS</span>
              <span className="text-xs text-muted-foreground">• Compliance & Gates</span>
            </div>
            <div className="flex items-center space-x-2">
              <span className="font-bold text-lg">PRODUCT MANAGERS</span>
              <span className="text-xs text-muted-foreground">• Traceability</span>
            </div>
          </div>
        </div>
      </section>

      {/* 6. Media Section (Visual Demonstration) */}
      <section id="how-it-works" className="border-y border-border/80 bg-card/40 py-20 backdrop-blur-sm">
        <div className="mx-auto max-w-7xl px-6">
          <div className="text-center">
            <h2 className="text-3xl font-bold tracking-tight sm:text-4xl">Visual Workflow & Runner Engine</h2>
            <p className="mt-4 text-muted-foreground max-w-2xl mx-auto">
              Inspect steps, review generated artifacts, approve progression, and view live execution logs.
            </p>
          </div>

          {/* Interactive Mock UI */}
          <div className="panel-shadow mt-12 overflow-hidden rounded-2xl border border-border/80 bg-[#0c0d12]">
            <div className="flex items-center justify-between border-b border-border/45 bg-[#0e1017] px-4 py-3 shrink-0">
              <div className="flex items-center space-x-2">
                <span className="h-3 w-3 rounded-full bg-red-500/80"></span>
                <span className="h-3 w-3 rounded-full bg-yellow-500/80"></span>
                <span className="h-3 w-3 rounded-full bg-green-500/80"></span>
                <span className="ml-2 font-mono text-xs text-muted-foreground">Workspace Project: flowpilot-core</span>
              </div>
              <div className="flex items-center space-x-3">
                <span className="font-mono text-xs text-muted-foreground">Run ID: fp-8239a</span>
              </div>
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-3 min-h-[550px] bg-[#0c0d12] text-foreground">
              {/* Left Sidebar */}
              <div className="border-r border-border/10 p-4 bg-[#0e1017] space-y-4 flex flex-col">
                <div className="flex items-center justify-between pb-2 border-b border-border/10 shrink-0">
                  <div className="flex items-center gap-2">
                    <div className="flex h-5 w-5 items-center justify-center rounded-lg bg-emerald-500/20 text-emerald-400 font-bold text-[10px] border border-emerald-500/30">
                      FP
                    </div>
                    <span className="font-bold tracking-tight text-sm text-foreground">FlowPilot</span>
                  </div>
                  <MoreVertical className="h-3.5 w-3.5 text-muted-foreground" />
                </div>

                <div className="shrink-0">
                  <h2 className="text-[10px] font-bold tracking-widest text-muted-foreground/60 uppercase">
                    PIPELINE STEPS
                  </h2>
                </div>

                <div className="space-y-2 flex-1">
                  {/* Step 1 */}
                  <div className="w-full flex items-center justify-between p-3 rounded-xl border border-transparent bg-transparent opacity-85">
                    <div className="min-w-0 flex-1">
                      <p className="text-[9px] font-mono text-muted-foreground uppercase tracking-wider">Step 1</p>
                      <h3 className="text-xs font-semibold text-foreground truncate mt-0.5">Business Intake</h3>
                      <p className="text-[9px] text-emerald-400 uppercase mt-0.5">COMPLETED</p>
                    </div>
                    <Check className="h-3.5 w-3.5 text-emerald-400 bg-emerald-950/40 rounded-full p-0.5 border border-emerald-500/20" />
                  </div>

                  {/* Step 2 (Active - Waiting Safe Gate Approval) */}
                  <div className="w-full flex items-center justify-between p-3 rounded-xl border border-emerald-500/30 bg-[#161d28] shadow-sm">
                    <div className="min-w-0 flex-1">
                      <p className="text-[9px] font-mono text-muted-foreground uppercase tracking-wider">Step 2</p>
                      <h3 className="text-xs font-bold text-accent truncate mt-0.5">Tech Spec Design</h3>
                      <p className="text-[9px] text-amber-400 uppercase mt-0.5 font-semibold">WAITING APPROVAL</p>
                    </div>
                    <ShieldAlert className="h-3.5 w-3.5 text-amber-500 animate-pulse" />
                  </div>

                  {/* Step 3 */}
                  <div className="w-full flex items-center justify-between p-3 rounded-xl border border-transparent bg-transparent opacity-50">
                    <div className="min-w-0 flex-1">
                      <p className="text-[9px] font-mono text-muted-foreground uppercase tracking-wider">Step 3</p>
                      <h3 className="text-xs font-semibold text-foreground truncate mt-0.5">TDD Code Signatures</h3>
                      <p className="text-[9px] text-muted-foreground uppercase mt-0.5">PENDING</p>
                    </div>
                    <div className="h-3 w-3 rounded-full border-2 border-muted-foreground/30" />
                  </div>

                  {/* Step 4 */}
                  <div className="w-full flex items-center justify-between p-3 rounded-xl border border-transparent bg-transparent opacity-50">
                    <div className="min-w-0 flex-1">
                      <p className="text-[9px] font-mono text-muted-foreground uppercase tracking-wider">Step 4</p>
                      <h3 className="text-xs font-semibold text-foreground truncate mt-0.5">Autonomous Coding</h3>
                      <p className="text-[9px] text-muted-foreground uppercase mt-0.5">PENDING</p>
                    </div>
                    <div className="h-3 w-3 rounded-full border-2 border-muted-foreground/30" />
                  </div>
                </div>
              </div>

              {/* Right Details Workspace */}
              <div className="lg:col-span-2 p-6 space-y-5 bg-[#0c0d12] overflow-y-auto flex flex-col justify-between relative">
                
                {/* Top close button */}
                <div className="absolute top-6 right-6 flex items-center gap-2">
                  <span className="text-xs text-muted-foreground">?</span>
                  <X className="h-4 w-4 text-muted-foreground" />
                </div>

                <div className="space-y-4">
                  {/* Header */}
                  <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border/10 pb-3 pr-12">
                    <div>
                      <span className="text-[9px] font-bold text-accent bg-accent/10 border border-accent/20 px-1.5 py-0.5 rounded uppercase tracking-widest">
                        STEP 2
                      </span>
                      <h2 className="text-lg font-bold tracking-tight text-foreground flex items-center gap-2.5 flex-wrap mt-0.5">
                        Tech Spec Design
                        <span className="bg-amber-500/10 text-amber-400 border border-amber-500/20 px-2 py-0.5 rounded-full text-[10px] font-semibold uppercase tracking-wider animate-pulse">
                          Awaiting Safe-Gate Approval
                        </span>
                      </h2>
                    </div>

                    {/* Actions */}
                    <div className="flex items-center gap-2 shrink-0">
                      <Button variant="secondary" size="sm" className="h-7 rounded-lg px-2 text-[10px] font-semibold bg-accent/20 border border-accent/30 text-accent" disabled>
                        <Play className="mr-1 h-3 w-3 fill-current" /> Resume
                      </Button>
                      <div className="h-3 w-[1px] bg-border/20" />
                      <div className="flex items-center gap-1.5 rounded-full border border-border bg-card/60 px-2 py-0.5 text-[9px]">
                        <span className="text-muted-foreground uppercase">YOLO</span>
                        <div className="relative inline-flex h-3 w-5 rounded-full bg-muted">
                          <span className="absolute left-0.5 top-0.5 h-2 w-2 rounded-full bg-background" />
                        </div>
                      </div>
                    </div>
                  </div>

                  {/* Prompt aligned to the right */}
                  <div className="flex justify-end">
                    <div className="bg-[#1b2b24] border border-emerald-500/20 p-3 rounded-xl space-y-0.5 max-w-[80%] text-left">
                      <span className="text-[8px] font-bold text-emerald-400 tracking-wider uppercase font-mono">
                        Initial Prompt
                      </span>
                      <p className="text-xs text-foreground leading-normal font-sans">
                        Generate structured technical plan for flowpilot-core
                      </p>
                    </div>
                  </div>

                  {/* Session Status & Output Badge */}
                  <div className="space-y-2">
                    <p className="text-[10px] font-mono text-muted-foreground flex items-center gap-1.5 flex-wrap">
                      <span>Session:</span>
                      <span className="bg-accent/40 text-accent-foreground px-1 py-0.5 rounded font-bold uppercase tracking-wider text-[8px]">
                        Shared Main
                      </span>
                      <span>&bull;</span>
                      <span className="text-foreground/90 font-sans">
                        Claude-3.5-Sonnet (claude-3-5-sonnet)
                      </span>
                      <span>&bull;</span>
                      <span className="font-semibold text-emerald-400">
                        active
                      </span>
                      <span>&bull;</span>
                      <span className="font-mono text-[8px] text-foreground/60">
                        019e66f7-afd9-7f62-937a-49a402ab6ffc
                      </span>
                    </p>

                    <div>
                      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full border border-emerald-500/20 bg-emerald-500/5 text-[9px] font-semibold uppercase text-emerald-400">
                        <FileText className="h-3 w-3" />
                        Artifact Generated
                      </span>
                    </div>
                  </div>

                  {/* Tabs */}
                  <div className="flex border border-border/30 bg-[#090a0f] rounded-lg p-0.5 gap-1.5 w-fit shrink-0">
                    <button type="button" style={{ fontSize: "7.5px" }} className="px-3 py-1 font-extrabold uppercase tracking-wider text-emerald-400 bg-emerald-500/5 rounded-md relative">
                      RESPONSE
                      <span className="absolute bottom-0 left-1/2 -translate-x-1/2 w-6 h-[1.5px] bg-emerald-400 rounded-full" />
                    </button>
                    <button type="button" style={{ fontSize: "7.5px" }} className="px-3 py-1 font-extrabold uppercase tracking-wider text-muted-foreground hover:text-foreground rounded-md animate-none">
                      PROMPT
                    </button>
                    <button type="button" style={{ fontSize: "7.5px" }} className="px-3 py-1 font-extrabold uppercase tracking-wider text-muted-foreground hover:text-foreground rounded-md animate-none">
                      ARTIFACT
                    </button>
                  </div>

                  {/* Output Preview */}
                  <div className="relative rounded-xl border border-border/45 bg-[#090a0f] p-4 overflow-hidden">
                    <div className="absolute top-3 right-3 text-muted-foreground">
                      <Copy className="h-3 w-3" />
                    </div>
                    <article className="whitespace-pre-wrap text-[#d1d5db] font-mono text-xs leading-relaxed break-words">
                      {`# Architecture Overview\n\n* Data persistence via Supabase Postgres.\n* Session state managed through local LiveSession adapters.\n\n# DB Schema Alterations\n\n* Create table workflow_run_sessions mapping process keys.`}
                    </article>
                    <div className="flex items-center justify-center pt-2">
                      <span className="text-[10px] font-bold text-accent uppercase tracking-wider hover:underline cursor-pointer">
                        Show more
                      </span>
                    </div>
                  </div>

                  {/* Collapsible Diagnostics */}
                  <div className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground flex items-center justify-between border-t border-border/10 pt-2 cursor-pointer">
                    <span>DEVELOPER DIAGNOSTICS</span>
                    <span className="font-mono text-[8px] opacity-60">Expand</span>
                  </div>
                </div>

                {/* Run Logs Card (Always-Visible Chat & Collapsed logs list) */}
                <div className="mt-4 border border-border/20 bg-[#11131c] rounded-xl overflow-hidden shadow-md">
                  <div className="flex items-center justify-between px-4 py-2 bg-[#0e1017]/60 border-b border-border/10">
                    <div className="flex items-center gap-1.5">
                      <FileText className="h-3.5 w-3.5 text-emerald-400" />
                      <span className="text-xs font-bold text-foreground">Run Logs</span>
                    </div>
                    <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
                  </div>

                  {/* Mock logs list */}
                  <div className="p-4 bg-[#11131c]/40 border-b border-border/10 font-mono text-[9px] text-muted-foreground/80 space-y-1">
                    <div>[07:10:01] Loading project bindings for UUID: flowpilot-core</div>
                    <div>[07:10:02] Analyzing directory structure...</div>
                  </div>

                  {/* Always-visible Safe Gate section inside logs container */}
                  <div className="p-4 bg-[#11131c]">
                    <div className="border border-border/80 bg-card/45 p-4 rounded-xl space-y-3">
                      <div className="flex items-center gap-1.5 pb-1.5 border-b border-border/30">
                        <ShieldAlert className="h-4 w-4 text-warning shrink-0 animate-pulse" />
                        <span className="text-xs font-semibold text-warning">
                          Safe Gate: Approving compiles files & begins coding stage
                        </span>
                      </div>

                      <div className="space-y-2">
                        <textarea
                          className="w-full min-h-[50px] resize-none bg-[#090a0f] border border-border/60 rounded-lg px-3 py-2 text-xs focus:outline-none placeholder:text-muted-foreground/60 text-[#eaeaea]"
                          placeholder="Provide revision notes for Reject & Retry..."
                          disabled
                          rows={2}
                        />

                        <div className="flex items-center justify-end gap-2 pt-1">
                          <Button
                            variant="outline"
                            className="rounded-lg px-3 h-8 border-border/80 hover:bg-muted text-foreground text-[10px] font-semibold"
                            disabled
                          >
                            Reject & Retry
                          </Button>
                          <Button
                            className="rounded-lg px-3 h-8 bg-emerald-600 text-white hover:bg-emerald-700 border-none flex items-center gap-1 text-[10px] font-semibold"
                            disabled
                          >
                            Approve & Continue &rarr;
                          </Button>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

              </div>
            </div>
          </div>
        </div>
      </section>

      {/* 7. Core Benefits (The 4 Superpowers Detailed) */}
      <section id="superpowers" className="mx-auto max-w-7xl px-6 py-24">
        <div className="text-center">
          <h2 className="text-4xl font-extrabold tracking-tight sm:text-5xl">The 4 Superpowers</h2>
          <p className="mt-4 text-muted-foreground max-w-3xl mx-auto text-base sm:text-lg">
            FlowPilot resolves the typical execution flaws of modern coding agents by acting as an un-bypassable runtime harness.
          </p>
        </div>

        <div className="mt-16 grid grid-cols-1 gap-8 md:grid-cols-2">
          {/* Card 1 */}
          <div className="panel-shadow rounded-2xl border border-border/80 bg-card p-8 hover:-translate-y-1 transition-all duration-300 hover:border-accent/40">
            <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-accent text-accent-foreground">
              <ShieldCheck className="h-6 w-6" />
            </div>
            <h3 className="mt-6 text-xl font-bold">Guaranteed Rule Adherence</h3>
            <p className="mt-3 text-muted-foreground text-sm leading-relaxed">
              Never let the AI skip validation, ignore rules, or skip steps. By orchestrating your workflow in isolated steps executed sequentially by a local runner, FlowPilot forces compliance. The AI cannot advance until the runner verifies step completion.
            </p>
            <ul className="mt-4 space-y-2 text-xs font-mono text-accent">
              <li className="flex items-center"><Check className="h-3.5 w-3.5 mr-2" /> Isolated execution loops per step</li>
              <li className="flex items-center"><Check className="h-3.5 w-3.5 mr-2" /> Non-bypassable step checklist verification</li>
            </ul>
          </div>

          {/* Card 2 */}
          <div className="panel-shadow rounded-2xl border border-border/80 bg-card p-8 hover:-translate-y-1 transition-all duration-300 hover:border-accent/40">
            <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-accent text-accent-foreground">
              <Key className="h-6 w-6" />
            </div>
            <h3 className="mt-6 text-xl font-bold">Un-bypassable Safe Gates</h3>
            <p className="mt-3 text-muted-foreground text-sm leading-relaxed">
              Give your teams control when executing code modifications or database operations. Safe Gates completely lock pipeline progression in the DB, requiring a physical, authenticated developer click to continue.
            </p>
            <ul className="mt-4 space-y-2 text-xs font-mono text-accent">
              <li className="flex items-center"><Check className="h-3.5 w-3.5 mr-2" /> Webhook-based runtime authorization</li>
              <li className="flex items-center"><Check className="h-3.5 w-3.5 mr-2" /> Permanent audit-logged sign-off trails</li>
            </ul>
          </div>

          {/* Card 3 */}
          <div className="panel-shadow rounded-2xl border border-border/80 bg-card p-8 hover:-translate-y-1 transition-all duration-300 hover:border-accent/40">
            <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-accent text-accent-foreground">
              <Cpu className="h-6 w-6" />
            </div>
            <h3 className="mt-6 text-xl font-bold">Advanced Context Harnessing</h3>
            <p className="mt-3 text-muted-foreground text-sm leading-relaxed">
              No more copying entire repositories. FlowPilot aggregates files, user context, and third-party MCP endpoints (Jira/Figma). It uses semantic vector search to rank and pack only the relevant data, automatically cutting out redundant tokens.
            </p>
            <ul className="mt-4 space-y-2 text-xs font-mono text-accent">
              <li className="flex items-center"><Check className="h-3.5 w-3.5 mr-2" /> pgvector-based artifact memory search</li>
              <li className="flex items-center"><Check className="h-3.5 w-3.5 mr-2" /> Auto-token compression & context sizing</li>
            </ul>
          </div>

          {/* Card 4 */}
          <div className="panel-shadow rounded-2xl border border-border/80 bg-card p-8 hover:-translate-y-1 transition-all duration-300 hover:border-accent/40">
            <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-accent text-accent-foreground">
              <Sparkles className="h-6 w-6" />
            </div>
            <h3 className="mt-6 text-xl font-bold">Self-Correction & Autonomous Loops</h3>
            <p className="mt-3 text-muted-foreground text-sm leading-relaxed">
              Work fully automated with YOLO mode. When errors occur, FlowPilot captures build warnings or compiler details, automatically updates skill files, and learns to avoid stuck loops and incorrect coding strategies.
            </p>
            <ul className="mt-4 space-y-2 text-xs font-mono text-accent">
              <li className="flex items-center"><Check className="h-3.5 w-3.5 mr-2" /> Automatic mistake-to-skill distillation</li>
              <li className="flex items-center"><Check className="h-3.5 w-3.5 mr-2" /> Wrong-way loop & repetition detection</li>
            </ul>
          </div>
        </div>
      </section>

      {/* Testimonials Section Removed */}

      {/* 9. FAQ Section */}
      <section id="faq" className="mx-auto max-w-4xl px-6 py-24">
        <div className="text-center">
          <h2 className="text-3xl font-bold tracking-tight">Frequently Asked Questions</h2>
          <p className="mt-4 text-muted-foreground">Everything you need to know about the FlowPilot harness.</p>
        </div>

        <div className="mt-12 space-y-4">
          {faqs.map((faq, index) => (
            <div
              key={index}
              className="panel-shadow rounded-2xl border border-border bg-card overflow-hidden transition-all duration-200"
            >
              <button
                onClick={() => toggleFaq(index)}
                className="flex w-full items-center justify-between px-6 py-4 text-left font-semibold text-foreground focus:outline-none hover:bg-muted/50"
              >
                <span>{faq.q}</span>
                {activeFaq === index ? (
                  <ChevronUp className="h-5 w-5 text-accent" />
                ) : (
                  <ChevronDown className="h-5 w-5 text-accent" />
                )}
              </button>

              {activeFaq === index && (
                <div className="px-6 pb-6 pt-2 text-sm text-muted-foreground leading-relaxed border-t border-border/40">
                  {faq.a}
                </div>
              )}
            </div>
          ))}
        </div>
      </section>

      {/* 10. Final CTA */}
      <section className="mx-auto max-w-7xl px-6 pb-24">
        <div className="rounded-3xl bg-accent text-accent-foreground p-12 text-center shadow-2xl relative overflow-hidden">
          <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_right,_rgba(255,255,255,0.15),_transparent_40%)]"></div>
          <h2 className="text-3xl font-extrabold sm:text-4xl md:text-5xl">
            Tame the AI. Supercharge the output.
          </h2>
          <p className="mx-auto mt-6 max-w-2xl text-accent-foreground/80 text-base sm:text-lg">
            Create structured pipelines with un-bypassable rules, safe gates, and optimized token memory today.
          </p>
          <div className="mt-8 flex flex-col sm:flex-row items-center justify-center gap-4">
            <Link to={session ? "/dashboard" : "/login"}>
              <Button className="bg-background text-foreground hover:bg-background/95 px-8 py-4 shadow-xl">
                {session ? "Go to Dashboard" : "Get Started Now"}
              </Button>
            </Link>
            <a href="#how-it-works">
              <Button variant="secondary" className="border-accent-foreground/30 hover:bg-accent-foreground/10 px-8 py-4 text-accent-foreground">
                Read Tech Specs
              </Button>
            </a>
          </div>
        </div>
      </section>

      {/* 11. Footer */}
      <footer className="border-t border-border/80 bg-card py-12 text-muted-foreground text-sm">
        <div className="mx-auto max-w-7xl px-6 grid grid-cols-1 md:grid-cols-4 gap-8">
          <div>
            <div className="flex items-center space-x-2">
              <Cpu className="h-5 w-5 text-accent" />
              <span className="font-mono text-base font-bold text-foreground">FlowPilot</span>
            </div>
            <p className="mt-4 text-xs">
              The control and execution harness for modern software engineering workflows. Built on Vite + Supabase + Go.
            </p>
          </div>
          <div>
            <h4 className="font-bold text-foreground mb-3 uppercase tracking-wider text-xs">Platform</h4>
            <ul className="space-y-2 text-xs">
              <li><a href="#superpowers" className="hover:text-accent">Features</a></li>
              <li><a href="#how-it-works" className="hover:text-accent">Timeline & Builder</a></li>
              <li><a href="#faq" className="hover:text-accent">FAQ</a></li>
            </ul>
          </div>
          <div>
            <h4 className="font-bold text-foreground mb-3 uppercase tracking-wider text-xs">Security</h4>
            <ul className="space-y-2 text-xs">
              <li><a href="#" className="hover:text-accent">Safe Gates</a></li>
              <li><a href="#" className="hover:text-accent">Row Level Security (RLS)</a></li>
              <li><a href="#" className="hover:text-accent">Local Workspace Isolation</a></li>
            </ul>
          </div>
          <div>
            <h4 className="font-bold text-foreground mb-3 uppercase tracking-wider text-xs">Product Info</h4>
            <p className="text-xs">
              FlowPilot is currently in active development. Registered trademark of FlowPilot Inc. 2026. All rights reserved.
            </p>
          </div>
        </div>
      </footer>
    </div>
  );
}

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { type ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { GoogleDriveProviderConfigCard } from './-GoogleDriveProviderConfigCard';
import type {
  GoogleDriveWorkspaceConfigResponse,
  GoogleDriveMcpProviderConfigStatus,
} from '@/domain/model/entity/local-runner';

// ── Mocks ─────────────────────────────────────────────────────────────────

const mockProviderConfigs: GoogleDriveMcpProviderConfigStatus[] = [
  {
    providerKey: 'codex',
    accountHomePath: '/home/user/.codexHome',
    configPath: '/home/user/.codexHome/config.toml',
    status: 'not_started',
    lastCheckedAt: new Date().toISOString(),
  },
  {
    providerKey: 'gemini',
    accountHomePath: '/home/user',
    configPath: '/home/user/.gemini/settings.json',
    status: 'configured',
    lastCheckedAt: new Date().toISOString(),
  },
  {
    providerKey: 'claude',
    accountHomePath: '/home/user',
    configPath: '/home/user/.claude.json',
    status: 'config_stale',
    lastCheckedAt: new Date().toISOString(),
    lastError: 'Credential paths are outdated',
  },
];

const mockGoogleDriveStatus: GoogleDriveWorkspaceConfigResponse = {
  artifactSync: {
    status: 'configured',
    source: 'local',
    configured: true,
    clientId: 'test-client-id',
    redirectUri: 'http://127.0.0.1:4317/...',
    hasClientSecret: true,
    hasPickerApiKey: false,
    missingFields: [],
  },
  mcp: {
    status: 'configured',
    configured: true,
    proxyMcpEnabled: true,
    credentialPath: '/home/user/.config/google-drive-mcp/gcp-oauth.keys.json',
    tokenPath: '/home/user/.config/google-drive-mcp/tokens.json',
    credentialFileExists: true,
    credentialFileValid: true,
    tokenFileExists: true,
    tokenRefreshValid: true,
    needsAuth: false,
    backendPackageAvailable: true,
    accountId: 'google-account-1',
    accountEmail: 'user@example.com',
    accountSelectionRequired: false,
    grantedScopes: [
      'https://www.googleapis.com/auth/drive.file',
      'https://www.googleapis.com/auth/drive.readonly',
      'openid',
      'email',
    ],
    missingScopes: [],
    accountReady: true,
    artifactBindingPresent: false,
    artifactReady: false,
    mcpReadReady: true,
    mcpWriteReady: true,
    reconnectRequired: false,
    missingFields: [],
  },
  accounts: [
    {
      accountId: 'google-account-1',
      accountEmail: 'user@example.com',
      status: 'connected',
      projectCount: 0,
      accountReady: true,
      mcpReadReady: true,
      mcpWriteReady: true,
      grantedScopes: [
        'https://www.googleapis.com/auth/drive.file',
        'https://www.googleapis.com/auth/drive.readonly',
        'openid',
        'email',
      ],
      missingScopes: [],
    },
  ],
  providerConfigs: mockProviderConfigs,
  runnerReachable: true,
  lastError: null,
};

const mockGoogleDriveStatusNotConfigured: GoogleDriveWorkspaceConfigResponse = {
  ...mockGoogleDriveStatus,
  artifactSync: {
    ...mockGoogleDriveStatus.artifactSync,
    hasClientSecret: false,
  },
  mcp: {
    ...mockGoogleDriveStatus.mcp,
    proxyMcpEnabled: true,
    configured: false,
    needsAuth: true,
    accountSelectionRequired: true,
    accountId: '',
    accountEmail: '',
    grantedScopes: [],
    missingScopes: [],
    accountReady: false,
    artifactBindingPresent: false,
    artifactReady: false,
    mcpReadReady: false,
    mcpWriteReady: false,
    reconnectRequired: false,
    status: 'needs_input',
  },
};

const mockGoogleDriveStatusProxyDisabled: GoogleDriveWorkspaceConfigResponse = {
  ...mockGoogleDriveStatus,
  mcp: {
    ...mockGoogleDriveStatus.mcp,
    proxyMcpEnabled: false,
  },
};

const mocks = vi.hoisted(() => ({
  ensureGoogleDriveMcpProviderConfig: vi.fn(),
}));

vi.mock('@/data/repository/browser-factory', () => ({
  createGatewayBundle: vi.fn(() => ({
    localRunnerGateway: {
      ensureGoogleDriveMcpProviderConfig: mocks.ensureGoogleDriveMcpProviderConfig,
    },
  })),
}));

// ── Test Suite ──────────────────────────────────────────────────────────────

describe('GoogleDriveProviderConfigCard', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    vi.clearAllMocks();
  });

  function renderComponent(status: GoogleDriveWorkspaceConfigResponse | null = mockGoogleDriveStatus) {
    return render(
      <QueryClientProvider client={queryClient}>
        <GoogleDriveProviderConfigCard googleDriveStatus={status} onStatusRefresh={vi.fn()} />
      </QueryClientProvider>
    );
  }

  describe('Card Structure', () => {
    it('renders the card title and description', () => {
      renderComponent();

      expect(screen.getByText('Proxy MCP provider setup')).toBeInTheDocument();
      expect(
        screen.getByText(/Configure Codex, Gemini, and Claude to use the FlowPilot proxy Google Drive MCP/)
      ).toBeInTheDocument();
    });

    it('displays all three provider badges when configs are available', () => {
      renderComponent();

      expect(screen.getByText('Codex')).toBeInTheDocument();
      expect(screen.getByText('Gemini')).toBeInTheDocument();
      expect(screen.getByText('Claude')).toBeInTheDocument();
    });

    it('shows message when no provider configs available', () => {
      const statusWithoutConfigs = { ...mockGoogleDriveStatus, providerConfigs: [] };
      renderComponent(statusWithoutConfigs);

      expect(screen.getByText(/No provider configurations available/)).toBeInTheDocument();
    });
  });

  describe('Status Badges', () => {
    it('displays correct status text for each provider', () => {
      renderComponent();

      // Codex is not_started -> "Not Started"
      expect(screen.getByText('Not Started')).toBeInTheDocument();
      // Gemini is configured -> "Configured"
      expect(screen.getByText('Configured')).toBeInTheDocument();
      // Claude is config_stale -> "Stale Config"
      expect(screen.getByText('Stale Config')).toBeInTheDocument();
    });

    it('displays provider details like account home path', () => {
      renderComponent();

      expect(screen.getByText(/Home: \/home\/user\/.codexHome/)).toBeInTheDocument();
      expect(screen.getByText(/Config: \/home\/user\/.codexHome\/config.toml/)).toBeInTheDocument();
    });

    it('displays error message for failed providers', () => {
      renderComponent();

      expect(screen.getByText(/Error: Credential paths are outdated/)).toBeInTheDocument();
    });
  });

  describe('Configure Button', () => {
    it('disables button when proxy Google Drive auth is not configured', () => {
      renderComponent(mockGoogleDriveStatusNotConfigured);

      const configButton = screen.getByRole('button', { name: /Configure AI Providers/ });
      expect(configButton).toBeDisabled();
    });

    it('shows informational message when proxy auth is not configured', () => {
      renderComponent(mockGoogleDriveStatusNotConfigured);

      expect(
        screen.getByText('Select an active Google account in Google Drive setup before configuring provider MCPs.')
      ).toBeInTheDocument();
    });

    it('explains missing proxy scopes when the selected account needs reconnect', () => {
      renderComponent({
        ...mockGoogleDriveStatus,
        mcp: {
          ...mockGoogleDriveStatus.mcp,
          status: 'needs_auth',
          configured: false,
          reconnectRequired: false,
          accountSelectionRequired: false,
          missingScopes: ['https://www.googleapis.com/auth/drive.readonly'],
          mcpReadReady: false,
        },
      });

      expect(
        screen.getByText(
          'Reconnect user@example.com and grant these scopes before configuring provider MCPs: https://www.googleapis.com/auth/drive.readonly.'
        )
      ).toBeInTheDocument();
      expect(
        screen.getByText(
          'Reconnect the selected Google account and grant the missing scopes: https://www.googleapis.com/auth/drive.readonly.'
        )
      ).toBeInTheDocument();
    });

    it('enables button when proxy mode is enabled and artifact-sync auth is ready even if legacy MCP status is not configured', () => {
      renderComponent({
        ...mockGoogleDriveStatus,
        mcp: {
          ...mockGoogleDriveStatus.mcp,
          proxyMcpEnabled: true,
          status: 'needs_oauth',
        },
      });

      const configButton = screen.getByRole('button', { name: /Configure AI Providers/ });
      expect(configButton).not.toBeDisabled();
    });

    it('disables button when proxy mode is off even if artifact-sync auth is ready', () => {
      renderComponent(mockGoogleDriveStatusProxyDisabled);

      const configButton = screen.getByRole('button', { name: /Configure AI Providers/ });
      expect(configButton).toBeDisabled();
    });

    it('keeps button enabled when all providers are already configured', () => {
      const allConfigured: GoogleDriveWorkspaceConfigResponse = {
        ...mockGoogleDriveStatus,
        providerConfigs: mockProviderConfigs.map((p) => ({ ...p, status: 'configured' })),
      };
      renderComponent(allConfigured);

      const configButton = screen.getByRole('button', { name: /Configure AI Providers/ });
      expect(configButton).not.toBeDisabled();
    });

    it('enables button when proxy MCP is configured and some providers need configuration', () => {
      renderComponent(mockGoogleDriveStatus);

      const configButton = screen.getByRole('button', { name: /Configure AI Providers/ });
      expect(configButton).not.toBeDisabled();
    });
  });

  describe('Configuration Flow', () => {
    it('calls ensure endpoint for each discovered provider when Configure button is clicked', async () => {
      mocks.ensureGoogleDriveMcpProviderConfig.mockResolvedValue({
        providerKey: 'codex',
        serverName: 'google-drive',
        status: 'configured',
        changed: true,
        configPath: '/home/user/.codexHome/config.toml',
      });

      renderComponent();

      const configButton = screen.getByRole('button', { name: /Configure AI Providers/ });
      fireEvent.click(configButton);

      await waitFor(() => {
        expect(mocks.ensureGoogleDriveMcpProviderConfig).toHaveBeenCalled();
      });

      expect(mocks.ensureGoogleDriveMcpProviderConfig.mock.calls).toHaveLength(3);
      expect(mocks.ensureGoogleDriveMcpProviderConfig.mock.calls[0]?.[0]).toMatchObject({
        providerKey: 'codex',
        accountHomePath: '/home/user/.codexHome',
      });
      expect(mocks.ensureGoogleDriveMcpProviderConfig.mock.calls[1]?.[0]).toMatchObject({
        providerKey: 'gemini',
        accountHomePath: '/home/user',
      });
      expect(mocks.ensureGoogleDriveMcpProviderConfig.mock.calls[2]?.[0]).toMatchObject({
        providerKey: 'claude',
        accountHomePath: '/home/user',
      });
    });

    it('shows loading state during configuration', async () => {
      mocks.ensureGoogleDriveMcpProviderConfig.mockImplementation(
        () => new Promise((resolve) => setTimeout(resolve, 100))
      );

      renderComponent();

      const configButton = screen.getByRole('button', { name: /Configure AI Providers/ });
      fireEvent.click(configButton);

      // Check that button shows loading indicator (text or spinner)
      await waitFor(() => {
        const button = screen.getByRole('button');
        const buttonText = button.textContent || '';
        // Button should be in loading state (disabled or with loading indicator)
        expect(button).toBeDisabled();
      });

      await waitFor(() => {
        const button = screen.getByRole('button');
        expect(button).not.toBeDisabled();
      });
    });

    it('shows success message after successful configuration', async () => {
      mocks.ensureGoogleDriveMcpProviderConfig.mockResolvedValue({
        providerKey: 'codex',
        serverName: 'google-drive',
        status: 'configured',
        changed: true,
        configPath: '/home/user/.codexHome/config.toml',
      });

      renderComponent();

      const configButton = screen.getByRole('button', { name: /Configure AI Providers/ });
      fireEvent.click(configButton);

      await waitFor(() => {
        expect(screen.getByText(/Applied configuration to 3 provider accounts successfully/)).toBeInTheDocument();
      });
    });

    it('shows error message if configuration fails', async () => {
      mocks.ensureGoogleDriveMcpProviderConfig.mockRejectedValueOnce(
        new Error('Configuration failed: invalid path')
      );

      renderComponent();

      const configButton = screen.getByRole('button', { name: /Configure AI Providers/ });
      fireEvent.click(configButton);

      await waitFor(() => {
        expect(screen.getByText(/Configuration failed/)).toBeInTheDocument();
      });
    });

    it('retries failed providers when they have a discovered account home', async () => {
      const failedStatus: GoogleDriveWorkspaceConfigResponse = {
        ...mockGoogleDriveStatus,
        providerConfigs: [
          {
            providerKey: 'codex',
            accountHomePath: '/home/user/.codexHome',
            configPath: '/home/user/.codexHome/config.toml',
            status: 'failed',
            lastCheckedAt: new Date().toISOString(),
            lastError: 'invalid TOML config',
          },
        ],
      };

      mocks.ensureGoogleDriveMcpProviderConfig.mockResolvedValue({
        providerKey: 'codex',
        serverName: 'google-drive',
        status: 'configured',
        changed: true,
        configPath: '/home/user/.codexHome/config.toml',
      });

      renderComponent(failedStatus);

      const configButton = screen.getByRole('button', { name: /Configure AI Providers/ });
      expect(configButton).not.toBeDisabled();
      fireEvent.click(configButton);

      await waitFor(() => {
        expect(mocks.ensureGoogleDriveMcpProviderConfig).toHaveBeenCalledWith(
          expect.objectContaining({
            providerKey: 'codex',
            accountHomePath: '/home/user/.codexHome',
          })
        );
      });
    });

    it('shows a partial success message when some providers still have no account home', async () => {
      const partiallyActionableStatus: GoogleDriveWorkspaceConfigResponse = {
        ...mockGoogleDriveStatus,
        providerConfigs: [
          {
            providerKey: 'codex',
            accountHomePath: '/home/user/.codexHome',
            configPath: '/home/user/.codexHome/config.toml',
            status: 'not_started',
            lastCheckedAt: new Date().toISOString(),
          },
          {
            providerKey: 'gemini',
            accountHomePath: '',
            configPath: '',
            status: 'not_started',
            lastCheckedAt: new Date().toISOString(),
          },
        ],
      };

      mocks.ensureGoogleDriveMcpProviderConfig.mockResolvedValue({
        providerKey: 'codex',
        serverName: 'google-drive',
        status: 'configured',
        changed: true,
        configPath: '/home/user/.codexHome/config.toml',
      });

      renderComponent(partiallyActionableStatus);

      fireEvent.click(screen.getByRole('button', { name: /Configure AI Providers/ }));

      await waitFor(() => {
        expect(
          screen.getByText(/Configured discovered provider accounts\. Some providers still need a discovered account home/)
        ).toBeInTheDocument();
      });
    });

    it('can refresh provider configs even when every provider is already configured', async () => {
      const allConfigured: GoogleDriveWorkspaceConfigResponse = {
        ...mockGoogleDriveStatus,
        providerConfigs: mockProviderConfigs.map((config) => ({
          ...config,
          status: 'configured',
        })),
      };

      mocks.ensureGoogleDriveMcpProviderConfig.mockResolvedValue({
        providerKey: 'codex',
        serverName: 'google-drive',
        status: 'configured',
        changed: true,
        configPath: '/home/user/.codexHome/config.toml',
      });

      renderComponent(allConfigured);

      fireEvent.click(screen.getByRole('button', { name: /Configure AI Providers/ }));

      await waitFor(() => {
        expect(mocks.ensureGoogleDriveMcpProviderConfig).toHaveBeenCalledTimes(3);
      });

      expect(mocks.ensureGoogleDriveMcpProviderConfig).toHaveBeenNthCalledWith(
        1,
        expect.objectContaining({ providerKey: 'codex', accountHomePath: '/home/user/.codexHome' })
      );
      expect(mocks.ensureGoogleDriveMcpProviderConfig).toHaveBeenNthCalledWith(
        2,
        expect.objectContaining({ providerKey: 'gemini', accountHomePath: '/home/user' })
      );
      expect(mocks.ensureGoogleDriveMcpProviderConfig).toHaveBeenNthCalledWith(
        3,
        expect.objectContaining({ providerKey: 'claude', accountHomePath: '/home/user' })
      );

      await waitFor(() => {
        expect(screen.getByText(/Refreshed 3 provider accounts successfully/)).toBeInTheDocument();
      });
    });
  });

  describe('Stale Config Display', () => {
    it('displays stale config status with warning appearance', () => {
      renderComponent();

      expect(screen.getByText('Stale Config')).toBeInTheDocument();
      // The component should display this with warning styling (yellow/orange badge)
    });

    it('shows error message for stale config', () => {
      renderComponent();

      expect(screen.getByText(/Error: Credential paths are outdated/)).toBeInTheDocument();
    });
  });

  describe('Null Status Handling', () => {
    it('renders safely when status is null', () => {
      renderComponent(null);

      expect(screen.getByText('Proxy MCP provider setup')).toBeInTheDocument();
      // Should show empty state or placeholder
      expect(screen.getByText(/No provider configurations available/)).toBeInTheDocument();
    });
  });
});

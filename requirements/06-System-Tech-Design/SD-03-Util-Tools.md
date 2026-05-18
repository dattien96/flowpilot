https://github.com/rtk-ai/rtk#how-it-works
https://github.com/abhigyanpatwari/GitNexus

2 above tools are open source, when user start use our product let see are they installed or not, if not installed then let prompt user to install them. -> got user confirmation for these tools and then call cobra golang tool to install them and continue the workflow.

# The install command:
## GitNexus
npm install -g gitnexus

## Rtk
### Homebrew (recommended)
brew install rtk
### Quick Install (Linux/macOS)
curl -fsSL https://raw.githubusercontent.com/rtk-ai/rtk/refs/heads/master/install.sh | sh
Installs to ~/.local/bin. Add to PATH if needed:

echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc  # or ~/.zshrc

## Verification
After installation, verify that the tools are available in your PATH by running:

gitnexus --version
rtk --version

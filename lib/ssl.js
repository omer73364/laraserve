const { execSync } = require("child_process");
const fs = require("fs");
const path = require("path");

function execCommand(command, description) {
  try {
    console.log(`   ${description}...`);
    execSync(command, { stdio: "inherit" });
  } catch (error) {
    throw new Error(`Failed to ${description.toLowerCase()}: ${error.message}`);
  }
}

function execAsUser(command, description) {
  try {
    console.log(`   ${description}...`);
    const uid = parseInt(process.env.SUDO_UID || process.getuid());
    const gid = parseInt(process.env.SUDO_GID || process.getgid());
    const home = process.env.SUDO_USER
      ? `/home/${process.env.SUDO_USER}`
      : process.env.HOME;

    execSync(command, {
      stdio: "inherit",
      uid: uid,
      gid: gid,
      env: {
        ...process.env,
        HOME: home,
        USER: process.env.SUDO_USER || process.env.USER,
      },
    });
  } catch (error) {
    throw new Error(`Failed to ${description.toLowerCase()}: ${error.message}`);
  }
}

function isMkcertInstalled() {
  try {
    execSync("which mkcert", { stdio: "ignore" });
    return true;
  } catch (error) {
    return false;
  }
}

function installMkcert() {
  console.log("\n📦 Installing mkcert...");

  // Update apt
  execCommand("apt update", "Updating apt");

  // Install dependencies
  execCommand("apt install -y libnss3-tools curl", "Installing dependencies");

  // Check if mkcert is already installed
  if (isMkcertInstalled()) {
    console.log("   ✓ mkcert already installed");
    return;
  }

  // Install mkcert
  console.log("   Downloading mkcert...");
  execSync('curl -JLO "https://dl.filippo.io/mkcert/latest?for=linux/amd64"', {
    stdio: "inherit",
  });
  execSync("chmod +x mkcert-v*-linux-amd64", { stdio: "ignore" });
  execSync("mv mkcert-v*-linux-amd64 /usr/local/bin/mkcert", {
    stdio: "ignore",
  });
  console.log("   ✓ mkcert installed successfully");
}

function setupSSL(domain) {
  console.log("\n🔒 Setting up SSL certificates...");

  // Ensure mkcert is installed
  if (!isMkcertInstalled()) {
    installMkcert();
  }

  // Install mkcert CA as the actual user (not root)
  // This ensures the CA is installed in the user's browser trust store
  try {
    console.log(
      "   Installing mkcert CA (this adds the certificate authority to your browser)..."
    );
    execAsUser("mkcert -install", "Installing mkcert CA as user");
    console.log("   ✓ CA certificate installed in user trust store");
  } catch (error) {
    console.log("   ⚠ CA might already be installed, continuing...");
  }

  // Create certificate directory
  const certDir = `/etc/ssl/${domain}`;
  if (!fs.existsSync(certDir)) {
    fs.mkdirSync(certDir, { recursive: true });
    console.log(`   ✓ Created certificate directory: ${certDir}`);
  }

  // Generate certificates as root (in /etc/ssl)
  const originalDir = process.cwd();
  try {
    // process.chdir(certDir);
    execAsUser(`mkcert ${domain}`, "Generating certificates");

    // move the files to the certificate directory
    execCommand(
      `mv ${domain}.pem /etc/ssl/${domain}/${domain}.pem`,
      `moving cert to /etc/ssl/${domain}`
    );
    execCommand(
      `mv ${domain}-key.pem /etc/ssl/${domain}/${domain}-key.pem`,
      `moving key to /etc/ssl/${domain}`
    );

    console.log(`   ✓ Certificates created in ${certDir}`);
  } finally {
    process.chdir(originalDir);
  }

  // Verify certificates exist
  const certFile = path.join(certDir, `${domain}.pem`);
  const keyFile = path.join(certDir, `${domain}-key.pem`);

  if (!fs.existsSync(certFile) || !fs.existsSync(keyFile)) {
    throw new Error("Certificate generation failed - files not found");
  }

  console.log(
    "   ✓ SSL setup completed - certificates are now trusted by your browser"
  );
}

module.exports = {
  setupSSL,
};

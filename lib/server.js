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

function enableApacheSite(domain, sslEnabled) {
  console.log("\n🔧 Enabling Apache site...");

  // Enable SSL module if SSL is enabled
  if (sslEnabled) {
    try {
      execCommand("a2enmod ssl", "Enabling SSL module");
    } catch (error) {
      console.log("   ⚠ SSL module might already be enabled, continuing...");
    }
  }

  // Enable the site
  execCommand(`a2ensite ${domain}.conf`, `Enabling site ${domain}`);

  // Reload Apache
  execCommand("systemctl reload apache2", "Reloading Apache");

  console.log("   ✓ Apache site enabled and reloaded");
}

function enableNginxSite(domain) {
  console.log("\n🔧 Enabling Nginx site...");

  const availablePath = `/etc/nginx/sites-available/${domain}`;
  const enabledPath = `/etc/nginx/sites-enabled/${domain}`;

  // Create symlink if it doesn't exist
  if (!fs.existsSync(enabledPath)) {
    try {
      fs.symlinkSync(availablePath, enabledPath);
      console.log(`   ✓ Created symlink to sites-enabled`);
    } catch (error) {
      throw new Error(`Failed to create symlink: ${error.message}`);
    }
  } else {
    console.log("   ✓ Symlink already exists");
  }

  // Test Nginx configuration
  try {
    execSync("nginx -t", { stdio: "inherit" });
  } catch (error) {
    throw new Error("Nginx configuration test failed");
  }

  // Reload Nginx
  execCommand("systemctl reload nginx", "Reloading Nginx");

  console.log("   ✓ Nginx site enabled and reloaded");
}

function enableSite(server, domain, sslEnabled) {
  if (server === "apache") {
    enableApacheSite(domain, sslEnabled);
  } else if (server === "nginx") {
    enableNginxSite(domain);
  } else {
    throw new Error(`Unsupported server type: ${server}`);
  }
}

module.exports = {
  enableSite,
};

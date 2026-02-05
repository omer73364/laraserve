const { execSync } = require("child_process");
const fs = require("fs");
const path = require("path");

function getWebServerUser(server) {
  if (server === "apache") {
    // Check for www-data (Debian/Ubuntu) or apache (RedHat/CentOS)
    try {
      execSync("id -u www-data", { stdio: "ignore" });
      return "www-data";
    } catch (error) {
      return "apache";
    }
  } else if (server === "nginx") {
    // Nginx typically runs as www-data or nginx
    try {
      execSync("id -u www-data", { stdio: "ignore" });
      return "www-data";
    } catch (error) {
      return "nginx";
    }
  }
  return "www-data"; // default
}

function fixPermissions(docRoot, server) {
  console.log("\n🔐 Setting up permissions...");

  const absolutePath = path.resolve(docRoot);

  // Verify path exists
  if (!fs.existsSync(absolutePath)) {
    throw new Error(`Document root does not exist: ${absolutePath}`);
  }

  const webUser = getWebServerUser(server);
  console.log(`   Web server user: ${webUser}`);

  // Get the actual user (not root)
  const actualUser = process.env.SUDO_USER || process.env.USER;

  try {
    // Set ownership to actual user and web server group
    console.log(`   Setting ownership to ${actualUser}:${webUser}...`);
    execSync(`chown -R ${actualUser}:${webUser} "${absolutePath}"`, {
      stdio: "inherit",
    });

    // Set directory permissions: 755 (rwxr-xr-x)
    // Owner can read/write/execute, group and others can read/execute
    console.log("   Setting directory permissions to 755...");
    execSync(`find "${absolutePath}" -type d -exec chmod 755 {} \\;`, {
      stdio: "inherit",
    });

    // Set file permissions: 666 (rw-r--r--)
    // Owner can read/write, group and others can read
    console.log("   Setting file permissions to 666...");
    execSync(`find "${absolutePath}" -type f -exec chmod 666 {} \\;`, {
      stdio: "inherit",
    });

    // If there's a storage directory (Laravel), make it writable by web server
    const storagePath = path.join(absolutePath, "storage");
    if (fs.existsSync(storagePath)) {
      console.log("   Setting storage directory permissions to 775...");
      execSync(`chmod -R 775 "${storagePath}"`, { stdio: "inherit" });
    }

    // If there's a bootstrap/cache directory (Laravel), make it writable
    const cachePath = path.join(absolutePath, "bootstrap", "cache");
    if (fs.existsSync(cachePath)) {
      console.log("   Setting cache directory permissions to 775...");
      execSync(`chmod -R 775 "${cachePath}"`, { stdio: "inherit" });
    }

    console.log("   ✓ Permissions configured successfully");
    console.log(`   ✓ Owner: ${actualUser}, Group: ${webUser}`);
  } catch (error) {
    throw new Error(`Failed to set permissions: ${error.message}`);
  }
}

module.exports = {
  fixPermissions,
};

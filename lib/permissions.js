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
  console.log("\n🔐 Setting up comprehensive permissions using ACL...");

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

    // Set default ACL for new files/directories with proper inheritance
    console.log("   Setting comprehensive default ACL rules...");
    execSync(`setfacl -R -d -m u::rwx,g::rwx,o::rx "${absolutePath}"`, {
      stdio: "inherit",
    });

    // Set initial ACL for existing directories
    console.log("   Setting directory permissions (rwxrwsr-x)...");
    execSync(`find "${absolutePath}" -type d -exec setfacl -m u::rwx,g::rwx,o::rx {} \\;`, {
      stdio: "inherit",
    });

    // Set initial ACL for existing files
    console.log("   Setting file permissions (rw-rw-r--)...");
    execSync(`find "${absolutePath}" -type f -exec setfacl -m u::rw,g::rw,o::r {} \\;`, {
      stdio: "inherit",
    });

    // Ensure all directories have the setgid bit for proper group inheritance
    console.log("   Ensuring proper group inheritance...");
    execSync(`find "${absolutePath}" -type d -exec chmod g+s {} \\;`, {
      stdio: "inherit",
    });

    // Make sure all files are group writable
    console.log("   Ensuring all files are group writable...");
    execSync(`find "${absolutePath}" -type f -exec chmod g+w {} \\;`, {
      stdio: "inherit",
    });

    // Special handling for Laravel storage directory
    const storagePath = path.join(absolutePath, "storage");
    if (fs.existsSync(storagePath)) {
      console.log("   Setting storage directory permissions (rwxrwsr-x)...");
      // Set setgid bit to ensure new files inherit the group
      execSync(`chmod g+s "${storagePath}"`, { stdio: "inherit" });
      // Set ACL for storage directory
      execSync(`setfacl -R -m u::rwx,g::rwx,o::rx "${storagePath}"`, { stdio: "inherit" });
      // Set default ACL for new files in storage
      execSync(`setfacl -R -d -m u::rwx,g::rwx,o::rx "${storagePath}"`, { stdio: "inherit" });
      
      // Special handling for SQLite database files
      const databasePath = path.join(storagePath, 'database');
      if (fs.existsSync(databasePath)) {
        console.log("   Ensuring SQLite database files are writable...");
        // Make database directory writable
        execSync(`setfacl -R -m u::rwx,g::rwx,o::rx "${databasePath}"`, { stdio: "inherit" });
        execSync(`setfacl -R -d -m u::rwx,g::rwx,o::rx "${databasePath}"`, { stdio: "inherit" });
        
        // Find and set permissions on all .sqlite files
        try {
          execSync(`find "${storagePath}" -name "*.sqlite*" -type f -exec chmod 664 {} \\;`, { stdio: "inherit" });
          execSync(`find "${storagePath}" -name "*.sqlite*" -type f -exec setfacl -m u::rw,g::rw,o::r {} \\;`, { stdio: "inherit" });
          console.log("   ✓ SQLite database permissions configured");
        } catch (error) {
          console.log("   ℹ️ No SQLite database files found or error setting permissions");
        }
      }
    }

    // Special handling for Laravel bootstrap/cache directory
    const cachePath = path.join(absolutePath, "bootstrap", "cache");
    if (fs.existsSync(cachePath)) {
      console.log("   Setting cache directory permissions (rwxrwsr-x)...");
      // Set setgid bit to ensure new files inherit the group
      execSync(`chmod g+s "${cachePath}"`, { stdio: "inherit" });
      // Set ACL for cache directory
      execSync(`setfacl -R -m u::rwx,g::rwx,o::rx "${cachePath}"`, { stdio: "inherit" });
      // Set default ACL for new files in cache
      execSync(`setfacl -R -d -m u::rwx,g::rwx,o::rx "${cachePath}"`, { stdio: "inherit" });
    }

    console.log("   ✓ Permissions configured successfully using ACL");
    console.log(`   ✓ Owner: ${actualUser}, Group: ${webUser}`);
    console.log("   ✓ ACL rules have been applied to ensure proper access control");
  } catch (error) {
    throw new Error(`Failed to set permissions: ${error.message}`);
  }
}

module.exports = {
  fixPermissions,
};

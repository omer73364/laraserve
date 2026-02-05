const { validateOptions, checkRootPrivileges } = require("./validator");
const { generateConfig } = require("./config");
const { setupSSL } = require("./ssl");
const { updateHostsFile } = require("./hosts");
const { enableSite } = require("./server");
const { fixPermissions } = require("./permissions");

async function setupSite(options) {
  console.log("\n🚀 Starting site setup...");
  console.log(`   Domain: ${options.domain}`);
  console.log(`   Path: ${options.path}`);
  console.log(`   Server: ${options.server}`);
  console.log(`   SSL: ${options.ssl ? "enabled" : "disabled"}`);

  try {
    // Step 1: Check root privileges
    checkRootPrivileges();

    // Step 2: Validate options
    validateOptions(options);

    // Step 3: Fix permissions on document root
    fixPermissions(options.path, options.server);

    // Step 4: Setup SSL if requested
    if (options.ssl) {
      setupSSL(options.domain);
    }

    // Step 5: Generate site configuration
    generateConfig(options);

    // Step 6: Update hosts file
    updateHostsFile(options.domain);

    // Step 7: Enable site and reload server
    enableSite(options.server, options.domain, options.ssl);
  } catch (error) {
    throw error;
  }
}

module.exports = {
  setupSite,
};

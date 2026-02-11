const { validateOptions, checkRootPrivileges, checkElevation } = require("./validator");
const { generateConfig } = require("./config");
const { setupSSL: platformSetupSSL } = require("./platform");
const { addEntry: platformAddEntry } = require("./platform");
const { enableSite: platformEnableSite } = require("./platform");
const { fixPermissions: platformFixPermissions, isAdmin, getElevationCommand, detectPlatform, Platform } = require("./platform");
const { saveSite } = require("./sites");
const { triggerError } = require("./utils");

async function setupSite(options) {
  console.log("\n🚀 Starting site setup...");
  console.log(`   Platform: ${process.platform}`);
  console.log(`   Domain: ${options.domain}`);
  console.log(`   Path: ${options.path}`);
  console.log(`   Server: ${options.server}`);
  console.log(`   SSL: ${options.ssl ? "enabled" : "disabled"}`);

  try {
    const platform = detectPlatform();

    if (platform === Platform.WINDOWS && !isAdmin()) {
      triggerError(`This command requires Administrator privileges.\n\n${getElevationCommand()}`);
      
    }

    checkRootPrivileges();
    checkElevation();

    validateOptions(options);

    await platformFixPermissions(options.path, options.server);

    let sslFiles = null;
    if (options.ssl) {
      sslFiles = await platformSetupSSL(options.domain);
    }

    await generateConfig(options, sslFiles);

    const hostsResult = await platformAddEntry(options.domain);
    console.log(`   ${hostsResult.message}`);

    await platformEnableSite(options.server, options.domain, options.ssl);

    saveSite({
      domain: options.domain,
      path: options.path,
      server: options.server,
      ssl: options.ssl,
      createdAt: new Date().toISOString(),
    });
  } catch (error) {
    triggerError(error);
    
  }
}

module.exports = {
  setupSite,
};

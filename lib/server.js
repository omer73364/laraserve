const { enableSite: platformEnableSite } = require("./platform");

async function enableSite(server, domain, sslEnabled) {
  await platformEnableSite(server, domain, sslEnabled);
}

module.exports = {
  enableSite,
};

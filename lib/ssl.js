const { setupSSL: platformSetupSSL } = require("./platform");

async function setupSSL(domain) {
  return await platformSetupSSL(domain);
}

module.exports = {
  setupSSL,
};

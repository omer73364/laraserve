const { fixPermissions: platformFixPermissions } = require("./platform");

async function fixPermissions(docRoot, server) {
  await platformFixPermissions(docRoot, server);
}

module.exports = {
  fixPermissions,
};

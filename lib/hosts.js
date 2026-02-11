const { addEntry: platformAddEntry } = require("./platform");

async function updateHostsFile(domain) {
  const result = await platformAddEntry(domain);
  console.log(`   ${result.message}`);
}

module.exports = {
  updateHostsFile,
};

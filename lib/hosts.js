const fs = require("fs");

const HOSTS_FILE = "/etc/hosts";

function updateHostsFile(domain) {
  console.log("\n📝 Updating hosts file...");

  // Read current hosts file
  let hostsContent;
  try {
    hostsContent = fs.readFileSync(HOSTS_FILE, "utf8");
  } catch (error) {
    throw new Error(`Failed to read hosts file: ${error.message}`);
  }

  // Check if entry already exists
  const pattern = new RegExp(`^127\\.0\\.0\\.1\\s+${domain}\\s*$`, "m");
  if (pattern.test(hostsContent)) {
    console.log(`   ✓ Entry for ${domain} already exists in hosts file`);
    return;
  }

  // Add entry
  const newEntry = `127.0.0.1 ${domain}`;
  const updatedContent = hostsContent.trim() + "\n" + newEntry + "\n";

  try {
    fs.writeFileSync(HOSTS_FILE, updatedContent, "utf8");
    console.log(`   ✓ Added ${domain} to hosts file`);
  } catch (error) {
    throw new Error(`Failed to write to hosts file: ${error.message}`);
  }
}

module.exports = {
  updateHostsFile,
};

const fs = require("fs"), path = require("path"), cp = require("child_process");
const root = "G:/projects/dssp/bank-repositories";
process.chdir(root);
const svcDir = "microservices/ms-balance-conversion-application-svc/src/main/java";
const libDirs = ["microservices/ms-lib-base-transaction","microservices/ms-workflow-base","microservices/ms-lib-base-domain-model","microservices/ms-lib-esb-sdk","microservices/ms-balance-conversion-application-sdk","microservices/ms-lib-common-base"];
let libSrc = "";
for (const d of libDirs) {
  cp.execSync(`find ${d} -name "*.java" -not -path "*test*"`).toString().split("\n").filter(Boolean)
    .forEach(f => { try { libSrc += fs.readFileSync(f, "utf8"); } catch (e) {} });
}
const expect = {
  transactionRecordITSStrategy: ["stampInProgress","commitWithRetry","getAndUpdateTransactionRecord","updateAppStatusUpdate"],
  sessionDataService: ["getSessionData","putSessionData"],
  domainRequestSynchronizationManager: ["getJwtSub","getSessionId","getCommonHttpHeader"],
  cardAccountSummaryService: ["getCreditCardAccountSummary"],
  baseTransactionIntegrationService: ["startTransaction","transitTransaction","getTransactionDetail","resumeTransactionDetail","getWorkflowTaskById","changeLanguage"],
  transactionApprovalInProgressService: ["processApprovalInProgress"],
  transactionAuditLogService: ["insertAuditDataToOcto"],
};
const files = cp.execSync(`find ${svcDir} -name "*.java"`).toString().split("\n").filter(Boolean);
let pass = 0, fail = 0;
const failLog = [];
for (const f of files) {
  const src = fs.readFileSync(f, "utf8");
  const ob = (src.match(/\{/g) || []).length, cb = (src.match(/\}/g) || []).length;
  if (ob !== cb) { failLog.push(`BRACE-FAIL ${f} (${ob} vs ${cb})`); fail++; } else pass++;
  const calls = src.match(/\b(transactionRecordITSStrategy|sessionDataService|domainRequestSynchronizationManager|cardAccountSummaryService|baseTransactionIntegrationService|transactionApprovalInProgressService|transactionAuditLogService)\.(\w+)\(/g) || [];
  for (const c of calls) {
    const bean = c.split(".")[0], m = c.match(/\.(\w+)\(/)[1];
    if (!expect[bean].includes(m)) { failLog.push(`CALL-UNEXPECTED ${path.basename(f)} ${c}`); fail++; }
    else {
      const re = new RegExp("public[^;]*\\b" + m + "\\s*\\(");
      if (!re.test(libSrc)) { failLog.push(`LIB-MISSING ${path.basename(f)} ${m}`); fail++; }
      else pass++;
    }
  }
}
console.log(failLog.join("\n"));
console.log(`structural: ${pass} PASS / ${fail} FAIL over ${files.length} java files`);

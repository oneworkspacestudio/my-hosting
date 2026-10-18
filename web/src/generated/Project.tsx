// Generated from screens/Project.aui; edit the AUI source.
import { Alert, Badge, Button, Card, Checkbox, Divider, Field, Grid, Heading, Icon, Input, Link, Metric, Page, Row, Select, Spacer, Stack, Text } from "@/components/ui"

export function Project({ data, onAction }: { data: any; onAction: (name: string) => void }) {
  return (
    <Page>
      <Link href="#projects" variant="breadcrumb">← All projects</Link>
      <Row variant="page-header">
        <Stack gap="sm">
          <Text variant="eyebrow">{data.project.adapter}</Text>
          <Heading level={1}>{data.project.name}</Heading>
          <Row>
            <Icon name="GitBranch" />
            <Text variant="muted">{data.project.repository}</Text>
            <Badge tone="muted">{data.project.branch}</Badge>
          </Row>
        </Stack>
        {data.canDeploy && (
          <Button onClick={() => onAction("triggerDialog")}>
            <Icon name="Rocket" />
            <Text>Trigger deploy</Text>
          </Button>
        )}
      </Row>
      <Row variant="tabs">
        {data.projectTabs.map((item, i) => (
          <Link key={i} href={item.href} variant={item.variant}>{item.title}</Link>
        ))}
      </Row>
      {data.tabOverview && (
        <>
          <Grid columns={2}>
            <Card variant="production">
              <Row>
                <Badge tone={data.productionTone}>{data.productionStatus}</Badge>
                <Spacer />
                <Icon name="Globe" />
              </Row>
              <Heading level={2}>Production deployment</Heading>
              <Text variant="muted">{data.productionMessage}</Text>
              {data.productionURL && (
                <Link href={data.productionURL} external={true} variant="button">Open site ↗</Link>
              )}
              <Text variant="caption">{data.productionCommit}</Text>
            </Card>
            <Card>
              <Heading level={2}>Project activity</Heading>
              <Text>{data.metricsLabel}</Text>
              <Grid columns={2}>
                <Stack>
                  <Metric value={data.metrics.requests} />
                  <Text variant="caption">Recorded requests</Text>
                </Stack>
                <Stack>
                  <Metric value={data.metrics.serverErrors} />
                  <Text variant="caption">Server errors</Text>
                </Stack>
              </Grid>
              <Text variant="caption">{data.metrics.scope}</Text>
            </Card>
          </Grid>
          <Row variant="section-header">
            <Heading level={2}>Recent deployments</Heading>
            <Link href={data.deploymentsHref}>View all →</Link>
          </Row>
          {data.recentDeploys.map((item, i) => (
            <Card key={i} href={item.href} variant="deployment">
              <Row>
                <Badge tone={item.tone}>{item.state}</Badge>
                <Stack gap="xs">
                  <Text>{item.title}</Text>
                  <Text variant="caption">{item.summary}</Text>
                </Stack>
                <Spacer />
                <Text variant="caption">{item.when}</Text>
              </Row>
            </Card>
          ))}
          {data.noDeploys && (
            <Card variant="empty">
              <Icon name="Rocket" />
              <Heading level={2}>Your first deploy starts here</Heading>
              <Text>Trigger a GitHub build or upload a ZIP containing your static output.</Text>
              {data.canDeploy && (
                <Button onClick={() => onAction("upload")}>Upload a ZIP</Button>
              )}
            </Card>
          )}
        </>
      )}
      {data.tabDeployments && (
        <>
          <Row variant="section-header">
            <Heading level={2}>Deployment history</Heading>
            <Select name="deployFilter" options={data.deployFilters} />
            {data.canDeploy && (
              <Button variant="secondary" onClick={() => onAction("upload")}>Upload ZIP</Button>
            )}
          </Row>
          {data.canDeploy && (
            <>
              <Field label="ZIP upload environment"><Select name="uploadEnvironment" options={data.environmentOptions} /></Field>
              <Text variant="caption">Preview uploads get their own HTTPS address and leave production serving its current release.</Text>
            </>
          )}
          {data.deployments.map((item, i) => (
            <Card key={i} href={item.href} variant="deployment">
              <Row>
                <Badge tone={item.tone}>{item.state}</Badge>
                <Stack gap="xs">
                  <Text>{item.title}</Text>
                  <Text variant="caption">{item.summary}</Text>
                </Stack>
                <Spacer />
                <Text variant="caption">{item.when}</Text>
              </Row>
            </Card>
          ))}
          {data.noFilteredDeploys && (
            <Card variant="empty"><Text>No deployments match this view.</Text></Card>
          )}
          <Row>
            <Button variant="secondary" onClick={() => onAction("previousPage")} disabled={data.firstPage}>← Previous</Button>
            <Text variant="caption">{data.pageLabel}</Text>
            <Button variant="secondary" onClick={() => onAction("nextPage")} disabled={data.lastPage}>Next →</Button>
          </Row>
        </>
      )}
      {data.tabBuild && (
        <>
          <Card variant="form">
            <Heading level={2}>Build and runtime settings</Heading>
            <Text variant="muted">Changes apply to new builds. Retry uses the original deployment's source and saved configuration; rebuild uses these current settings.</Text>
            <Field label="GitHub repository"><Input name="repository" /></Field>
            <Field label="Production branch"><Input name="branch" /></Field>
            {data.canAdmin && (
              <Button variant="secondary" onClick={() => onAction("detectFramework")}>Detect framework from repository</Button>
            )}
            <Field label="Adapter"><Select name="adapter" options={data.adapterOptions} /></Field>
            <Field label="Node major"><Select name="nodeMajor" options={data.nodeOptions} /></Field>
            <Field label="Install command · JSON arguments"><Input name="install" /></Field>
            <Field label="Build command · JSON arguments"><Input name="build" /></Field>
            <Field label="Start command · JSON arguments"><Input name="start" /></Field>
            <Field label="Base directory within the repository"><Input name="baseDirectory" placeholder=". or a package folder" /></Field>
            <Field label="Static output directory"><Input name="output" /></Field>
            <Field label="Health path"><Input name="healthPath" /></Field>
            <Grid columns={3}>
              <Field label="Timeout · seconds"><Input name="timeout" type="number" min={10} max={1800} /></Field>
              <Field label="RAM · MiB"><Input name="memoryMB" type="number" min={128} max={4096} /></Field>
              <Field label="Disk · MiB"><Input name="diskMB" type="number" min={10} max={10240} /></Field>
            </Grid>
            <Checkbox name="previews" label="Allow preview branches" />
            <Checkbox name="trusted" label="I approve this source and its executable scripts" />
            {data.canAdmin && (
              <Button onClick={() => onAction("saveBuild")}>Save build settings</Button>
            )}
          </Card>
          <Card variant="form">
            <Heading level={2}>GitHub connection & automatic deploys</Heading>
            <Text>{data.github.message}</Text>
            <Text variant="caption">{data.github.webhookURL}</Text>
            <Field label="Fine-grained GitHub token · Contents: read (optional for public repos)"><Input name="githubToken" type="password" autoComplete="off" /></Field>
            {data.canAdmin && (
              <Button onClick={() => onAction("connectGitHub")}>Verify repository & configure webhook</Button>
            )}
            {data.webhookSecret && (
              <>
                <Alert tone="info">Copy this webhook secret into GitHub now. It is shown only once.</Alert>
                <Text variant="code">{data.webhookSecret}</Text>
              </>
            )}
            <Text variant="caption">In GitHub repository Settings → Webhooks, add the URL above, JSON content type and this secret, selecting Push events. The URL must be reachable from GitHub.</Text>
          </Card>
          <Card variant="form">
            <Heading level={2}>Environment variables & secrets</Heading>
            <Text variant="muted">Values are encrypted on this Windows host and write-only after saving. Each deployment snapshots its environment; logs mask secret values.</Text>
            {data.secrets.map((item, i) => (
              <Row key={i}>
                <Badge tone="muted">{item.environment}</Badge>
                <Text variant="code">{item.name}</Text>
                <Spacer />
                <Text>••••••••</Text>
              </Row>
            ))}
            <Field label="Name"><Input name="secretName" placeholder="API_TOKEN" /></Field>
            <Field label="Environment"><Select name="secretEnvironment" options={data.environmentOptions} /></Field>
            <Field label="New value"><Input name="secretValue" type="password" autoComplete="off" /></Field>
            {data.canAdmin && (
              <Row>
                <Button onClick={() => onAction("saveSecret")}>Save protected value</Button>
                <Button variant="secondary" onClick={() => onAction("deleteSecret")}>Remove named value</Button>
              </Row>
            )}
          </Card>
        </>
      )}
      {data.tabDomains && (
        <>
          <Card variant="tip">
            <Heading level={2}>A separate origin for every site</Heading>
            <Text>{data.domainGuidance}</Text>
            <Link href={data.localURL} external={true}>Open local production URL ↗</Link>
          </Card>
          {data.domains.map((item, i) => (
            <Card key={i} href={item.href} variant="domain">
              <Row>
                <Icon name="Globe" />
                <Heading level={3}>{item.host}</Heading>
                <Spacer />
                <Badge tone={item.tone}>{item.status}</Badge>
              </Row>
              <Text variant="caption">{item.error}</Text>
            </Card>
          ))}
          <Card variant="form">
            <Heading level={2}>Add a custom domain</Heading>
            <Field label="Hostname"><Input name="domain" placeholder="app.example.com" /></Field>
            {data.canAdmin && (
              <Button onClick={() => onAction("addDomain")}>Add domain</Button>
            )}
            {data.selectedDomain && (
              <>
                <Divider />
                <Heading level={3}>{data.selectedDomain.host}</Heading>
                <Text>Create a TXT record at:</Text>
                <Text variant="code">{data.dnsRecord}</Text>
                <Text>With value:</Text>
                <Text variant="code">{data.selectedDomain.token}</Text>
                <Alert tone="info">{data.selectedDomain.error}</Alert>
                {data.canAdmin && (
                  <Row>
                    <Button onClick={() => onAction("verifyDomain")}>Check DNS & HTTPS</Button>
                    {data.canUseCanonical && (
                      <Button variant="secondary" onClick={() => onAction("makeCanonical")}>Make canonical address</Button>
                    )}
                    {data.selectedDomain.canonical && (
                      <Button variant="secondary" onClick={() => onAction("clearCanonical")}>Use managed site address</Button>
                    )}
                    <Button variant="danger" onClick={() => onAction("removeDomain")}>Remove binding</Button>
                  </Row>
                )}
              </>
            )}
          </Card>
        </>
      )}
      {data.tabProcess && (
        <>
          <Card>
            <Row>
              <Heading level={2}>Managed Node process</Heading>
              <Badge tone="muted">{data.process.state}</Badge>
            </Row>
            <Text>{data.process.error}</Text>
            <Text variant="caption">{data.processSummary}</Text>
            <Text variant="caption">{data.process.limits}</Text>
            {data.canDeploy && (
              <Row>
                <Button onClick={() => onAction("processStart")}>Start</Button>
                <Button variant="secondary" onClick={() => onAction("processRestart")}>Restart</Button>
                <Button variant="danger" onClick={() => onAction("processStop")}>Stop</Button>
              </Row>
            )}
          </Card>
          <Card variant="terminal">
            <Row>
              <Heading level={3}>Process logs</Heading>
              <Spacer />
              <Button variant="secondary" onClick={() => onAction("refreshProcessLogs")}>Refresh logs</Button>
            </Row>
            <Text variant="pre">{data.processLogs}</Text>
          </Card>
          <Alert tone="info">This is persistent Node hosting, not a managed serverless-functions service. Static sites do not have a Node process.</Alert>
        </>
      )}
      {data.tabAccess && (
        <>
          <Card>
            <Heading level={2}>Project access</Heading>
            <Text variant="muted">Owners and global administrators can manage all projects. Other members see only projects explicitly shared with them.</Text>
            {data.members.map((item, i) => (
              <Row key={i} variant="member">
                <Stack>
                  <Text>{item.name}</Text>
                  <Text variant="caption">{item.email}</Text>
                </Stack>
                <Spacer />
                <Badge tone="muted">{item.role}</Badge>
              </Row>
            ))}
          </Card>
          {data.canAdmin && (
            <Card variant="form">
              <Heading level={3}>Grant project access</Heading>
              <Field label="Signed-in MyAccounts member"><Select name="memberId" options={data.memberOptions} /></Field>
              <Field label="Project role"><Select name="memberRole" options={data.projectRoleOptions} /></Field>
              <Row>
                <Button onClick={() => onAction("saveMember")}>Save access</Button>
                <Button variant="secondary" onClick={() => onAction("removeMember")}>Remove access</Button>
              </Row>
            </Card>
          )}
        </>
      )}
      {data.tabAudit && (
        <>
          <Heading level={2}>Project audit trail</Heading>
          <Text variant="muted">Append-only API events with a SHA-256 chain. Administrators with direct filesystem/database access remain trusted.</Text>
          {data.audit.map((item, i) => (
            <Card key={i} variant="event">
              <Row>
                <Badge tone="muted">{item.action}</Badge>
                <Text variant="caption">{item.at}</Text>
              </Row>
              <Text variant="code">{item.summary}</Text>
            </Card>
          ))}
        </>
      )}
      {data.showTrigger && (
        <Card variant="dialog">
          <Heading level={2}>Deploy from GitHub</Heading>
          <Field label="Branch"><Input name="deployBranch" /></Field>
          <Field label="Commit SHA · optional; empty resolves branch HEAD"><Input name="deployCommit" placeholder="40-character commit SHA" /></Field>
          <Field label="Environment"><Select name="deployEnvironment" options={data.environmentOptions} /></Field>
          <Checkbox name="clearCache" label="Clear npm build cache before deploying" />
          <Row>
            <Button onClick={() => onAction("triggerDeploy")}>Trigger deploy</Button>
            <Button variant="secondary" onClick={() => onAction("closeTrigger")}>Cancel</Button>
          </Row>
        </Card>
      )}
    </Page>
  )
}

// Generated from screens/Settings.aui; edit the AUI source.
import { Alert, Badge, Button, Card, Checkbox, Field, Grid, Heading, Input, Link, Page, Row, Select, Spacer, Stack, Text } from "@/components/ui"

export function Settings({ data, onAction }: { data: any; onAction: (name: string) => void }) {
  return (
    <Page>
      <Row variant="page-header">
        <Stack gap="sm">
          <Text variant="eyebrow">YOUR INFRASTRUCTURE</Text>
          <Heading level={1}>Settings & health</Heading>
          <Text variant="muted">Manage native runtimes, routing, storage and recovery.</Text>
        </Stack>
        <Badge tone="muted">{data.version}</Badge>
      </Row>
      <Alert tone="info">{data.settings.securityScope}</Alert>
      <Grid columns={3}>
        {data.runtimes.map((item, i) => (
          <Card key={i}>
            <Row>
              <Heading level={2}>{item.name}</Heading>
              <Spacer />
              <Badge tone={item.tone}>{item.state}</Badge>
            </Row>
            <Text>{item.version}</Text>
            <Text variant="caption">{item.description}</Text>
          </Card>
        ))}
      </Grid>
      <Card>
        <Heading level={2}>Verified runtime provisioning</Heading>
        <Text variant="muted">Downloads use pinned official releases and SHA-256 checks. Static sites do not install Node. Offline import accepts only the exact pinned Windows AMD64 ZIP.</Text>
        <Field label="Runtime"><Select name="runtime" options={data.runtimeOptions} /></Field>
        <Row>
          <Button onClick={() => onAction("provisionRuntime")}>Download & verify runtime</Button>
          <Button variant="secondary" onClick={() => onAction("importRuntime")}>Import verified offline ZIP</Button>
        </Row>
        <Text variant="caption">{data.runtimeStatus}</Text>
      </Card>
      <Card variant="form">
        <Heading level={2}>Routing & retention</Heading>
        <Field label="Local site port"><Input name="caddyPort" type="number" min={1024} max={65535} /></Field>
        <Field label="Sites parent domain"><Input name="previewDomain" placeholder="sites.example.com" /></Field>
        <Field label="Site address style"><Select name="addressStyle" options={data.addressStyleOptions} /></Field>
        <Text variant="caption">Free tunnel: choose flat addresses and your root domain, giving my-site-sites.example.com. Direct HTTPS: choose nested addresses and sites.example.com. Previews add deploy-ID-- before the site name. Leave the domain empty for .localhost only.</Text>
        <Field label="Public HTTPS connection"><Select name="transport" options={data.transportOptions} /></Field>
        <Text variant="caption">Direct: point wildcard DNS at this server and open TCP 80/443. Tunnel: route wildcard DNS through your tunnel to the local site port, preserving the original hostname. Cloudflare Universal SSL does not cover nested sites.example.com hostnames; check edge certificate coverage before enabling public links.</Text>
        <Checkbox name="public" label="Enable public site addresses after DNS and HTTPS are ready" />
        <Field label="Retained release target"><Input name="retain" type="number" min={2} max={100} /></Field>
        <Text variant="caption">Published artifacts needed by live routes and rollback references are preserved. Cleanup is explicit; backups are never treated as build cache.</Text>
        <Field label="Concurrent builds"><Select name="concurrency" options={data.concurrencyOptions} /></Field>
        <Button onClick={() => onAction("saveSettings")}>Save routing settings</Button>
      </Card>
      <Card>
        <Heading level={2}>Protected storage</Heading>
        <Text variant="code">{data.settings.dataPath}</Text>
        <Text variant="muted">SQLite metadata, projects, release artifacts, logs, encrypted secrets, Caddy state and backups are stored in named folders. Windows persistent ACLs are required.</Text>
        <Text variant="caption">{data.caddySummary}</Text>
      </Card>
      <Card variant="form">
        <Heading level={2}>Encrypted backup & recovery</Heading>
        <Text variant="muted">Export a consistent, password-encrypted backup. Node processes pause during export and resume afterward. Secrets are portable inside the encrypted archive; browser sessions are excluded.</Text>
        <Field label="Backup encryption password"><Input name="backupPassword" type="password" autoComplete="new-password" /></Field>
        <Button onClick={() => onAction("exportBackup")}>Export encrypted backup</Button>
        <Text variant="caption">Keep the password separately. Restore offline using MyHosting.exe --backup BACKUP_FILE --restore-to NEW_NTFS_FOLDER --password-stdin restore. Existing installations are never overwritten. Re-provision runtimes and configure this host's OIDC callback/domain after moving machines.</Text>
      </Card>
      <Card>
        <Heading level={2}>Updates & background operation</Heading>
        <Text>Workspace installs compatible releases from the One Workspace Studio organization. Standalone users replace the stopped application with a verified release and retain the private-data folder.</Text>
        <Link href={data.releasesURL} external={true}>View verified release downloads ↗</Link>
        <Text variant="caption">For Windows services, use a dedicated non-system account with access only to the Hosting folder. Executable builds fail closed under LocalSystem. See the Windows service guide.</Text>
      </Card>
    </Page>
  )
}

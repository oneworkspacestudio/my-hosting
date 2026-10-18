// Generated from screens/Deploy.aui; edit the AUI source.
import { Alert, Badge, Button, Card, Grid, Heading, Icon, Link, Page, Row, Spacer, Stack, Text } from "@/components/ui"

export function Deploy({ data, onAction }: { data: any; onAction: (name: string) => void }) {
  return (
    <Page>
      <Link href={data.deploymentsHref} variant="breadcrumb">← Deployment history</Link>
      <Row variant="page-header">
        <Stack gap="sm">
          <Text variant="eyebrow">{data.deploy.environment}</Text>
          <Heading level={1}>{data.deployTitle}</Heading>
          <Text variant="muted">{data.deploySummary}</Text>
        </Stack>
        <Badge tone={data.deployTone}>{data.deploy.state}</Badge>
      </Row>
      <Grid columns={4} variant="metrics">
        <Card>
          <Text variant="caption">SOURCE</Text>
          <Text variant="code">{data.shortCommit}</Text>
          <Text variant="caption">{data.deploy.branch}</Text>
        </Card>
        <Card>
          <Text variant="caption">DURATION</Text>
          <Heading level={2}>{data.duration}</Heading>
          <Text variant="caption">{data.deploy.created}</Text>
        </Card>
        <Card>
          <Text variant="caption">OUTPUT</Text>
          <Heading level={2}>{data.outputSize}</Heading>
          <Text variant="caption">{data.deploy.config.adapter}</Text>
        </Card>
        <Card>
          <Text variant="caption">ENVIRONMENT</Text>
          <Heading level={2}>{data.deploy.environment}</Heading>
          <Text variant="caption">{data.nodeVersion}</Text>
        </Card>
      </Grid>
      {data.deploy.error && (
        <Alert tone="error">{data.deploy.error}</Alert>
      )}
      {data.deploy.url && (
        <Card variant="tip">
          <Row>
            <Icon name="Globe" />
            <Link href={data.deploy.url} external={true}>{data.deploy.url}</Link>
          </Row>
        </Card>
      )}
      <Row variant="actions">
        {data.canCancel && (
          <Button variant="danger" onClick={() => onAction("cancelDeploy")}>Cancel deployment</Button>
        )}
        {data.canRetry && (
          <Button onClick={() => onAction("retryDeploy")}>Retry same source & configuration</Button>
        )}
        {data.canRebuild && (
          <Button variant="secondary" onClick={() => onAction("rebuildDeploy")}>Rebuild with current settings</Button>
        )}
        {data.canRepublish && (
          <Button variant="secondary" onClick={() => onAction("republishDeploy")}>Republish this artifact</Button>
        )}
        {data.canRollback && (
          <Button variant="secondary" onClick={() => onAction("rollbackDeploy")}>Restore this production release</Button>
        )}
        {data.canPromote && (
          <Button onClick={() => onAction("promoteDeploy")}>Promote preview to production</Button>
        )}
        <Link href={data.downloadLogsURL} external={true} variant="button-secondary">Download logs</Link>
      </Row>
      <Card variant="terminal">
        <Row>
          <Heading level={3}>Deployment log</Heading>
          <Spacer />
          <Badge tone={data.streamTone}>{data.streamStatus}</Badge>
        </Row>
        <Text variant="pre">{data.buildLogs}</Text>
      </Card>
      <Card>
        <Heading level={2}>Exact build configuration</Heading>
        <Text variant="muted">This deployment keeps its own immutable source revision and configuration snapshot. Environment values are encrypted and never displayed.</Text>
        <Text variant="pre">{data.configJSON}</Text>
        <Text variant="caption">{data.sourceProof}</Text>
        <Text variant="caption">{data.artifactProof}</Text>
        <Text variant="caption">Test evidence: deployment health probes and configuration validation are recorded in the log; arbitrary project test suites run only if included in the project's build command.</Text>
      </Card>
    </Page>
  )
}

// Generated from screens/Projects.aui; edit the AUI source.
import { Badge, Button, Card, Divider, Grid, Heading, Icon, Metric, Page, Row, Spacer, Stack, Text } from "@/components/ui"

export function Projects({ data, onAction }: { data: any; onAction: (name: string) => void }) {
  return (
    <Page>
      <Row variant="page-header">
        <Stack gap="sm">
          <Text variant="eyebrow">BUILD SOMETHING GREAT</Text>
          <Heading level={1}>Your projects</Heading>
          <Text variant="muted">From source to a live site. Everything stays on your server.</Text>
        </Stack>
        {data.canAdmin && (
          <Button onClick={() => onAction("createProject")}>
            <Icon name="Plus" />
            <Text>Add new project</Text>
          </Button>
        )}
      </Row>
      <Grid columns={3} variant="metrics">
        <Card>
          <Text variant="caption">PROJECTS</Text>
          <Metric value={data.projectCount} />
          <Text variant="muted">Sites you can access</Text>
        </Card>
        <Card>
          <Text variant="caption">IN PRODUCTION</Text>
          <Metric value={data.liveCount} />
          <Text variant="muted">Published releases</Text>
        </Card>
        <Card>
          <Text variant="caption">DEPLOYMENT MODEL</Text>
          <Heading level={2}>Native Windows</Heading>
          <Text variant="muted">Caddy routing · optional Node runtimes</Text>
        </Card>
      </Grid>
      <Row variant="section-header">
        <Heading level={2}>All projects</Heading>
        <Badge tone="muted">{data.projectCount}</Badge>
      </Row>
      {data.emptyProjects && (
        <Card variant="empty">
          <Icon name="Layers" />
          <Heading level={2}>A home for your next idea</Heading>
          <Text>Connect a GitHub repository or upload a static site ZIP to publish your first project.</Text>
          {data.canAdmin ? (
            <Button onClick={() => onAction("createProject")}>Create a project</Button>
          ) : (
            <Text>Ask your Hosting administrator to grant you project access.</Text>
          )}
        </Card>
      )}
      <Grid columns={3} variant="projects">
        {data.projects.map((item, i) => (
          <Card key={i} href={item.href} variant="project">
            <Row>
              <Icon name={item.icon} />
              <Spacer />
              <Badge tone={item.tone}>{item.status}</Badge>
            </Row>
            <Heading level={2}>{item.name}</Heading>
            <Text variant="muted">{item.description}</Text>
            <Divider />
            <Row>
              <Icon name="GitBranch" />
              <Text variant="caption">{item.branch}</Text>
              <Spacer />
              <Text variant="caption">{item.adapter}</Text>
            </Row>
            <Text variant="caption">{item.repository}</Text>
          </Card>
        ))}
      </Grid>
      <Card variant="tip">
        <Row>
          <Icon name="ShieldCheck" />
          <Stack gap="xs">
            <Heading level={3}>Built for your own trusted projects</Heading>
            <Text>Build scripts run code. Only deploy sources approved by your administrator. The management dashboard and hosted sites use separate origins.</Text>
          </Stack>
        </Row>
      </Card>
    </Page>
  )
}

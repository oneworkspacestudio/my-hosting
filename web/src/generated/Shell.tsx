// Generated from screens/Shell.aui; edit the AUI source.
import { Alert, Badge, Button, Card, Heading, Icon, Image, Input, Link, Page, Row, Section, Spacer, Spinner, Stack, Text } from "@/components/ui"

export function Shell({ data, onAction }: { data: any; onAction: (name: string) => void }) {
  return (
    <Page>
      <Stack variant="sidebar">
        <Row variant="brand">
          <Image src={data.logo} alt="My Hosting" width={38} height={38} />
          <Stack gap="xs">
            <Heading level={2}>My Hosting</Heading>
            <Text variant="caption">ONE WORKSPACE</Text>
          </Stack>
        </Row>
        <Text variant="eyebrow">WORKSPACE</Text>
        {data.nav.map((item, i) => (
          <Link key={i} href={item.href} variant={item.variant}>
            <Icon name={item.icon} />
            <Text>{item.title}</Text>
          </Link>
        ))}
        <Spacer />
        <Card variant="scope">
          <Badge tone="success">Self hosted</Badge>
          <Text variant="caption">Your projects. Your server.</Text>
          <Text variant="caption">Trusted source only</Text>
        </Card>
        <Link href={data.workspaceURL} variant="nav">← One Workspace</Link>
      </Stack>
      <Stack variant="workspace">
        <Row variant="topbar">
          <Input name="search" placeholder="Search projects or jump to a site…" type="search" />
          <Button variant="icon" onClick={() => onAction("notifications")} aria-label="Deployment notifications"><Icon name="Bell" /></Button>
          <Button variant="icon" onClick={() => onAction("theme")} aria-label="Toggle color theme"><Icon name="SunMoon" /></Button>
          <Badge tone="muted">Windows · self hosted</Badge>
          <Row variant="profile">
            <Text>{data.initials}</Text>
            <Stack gap="xs">
              <Text>{data.user.name}</Text>
              <Text variant="caption">{data.user.role}</Text>
            </Stack>
          </Row>
        </Row>
        <Stack variant="content">
          {data.error && (
            <Alert tone="error">{data.error}</Alert>
          )}
          {data.notice && (
            <Alert tone="success">{data.notice}</Alert>
          )}
          {data.busy && (
            <Row>
              <Spinner />
              <Text>{data.busy}</Text>
            </Row>
          )}
          <Section id="screen-outlet" />
        </Stack>
      </Stack>
    </Page>
  )
}

// Generated from screens/Team.aui; edit the AUI source.
import { Alert, Badge, Button, Card, Field, Heading, Icon, Page, Row, Select, Spacer, Stack, Text } from "@/components/ui"

export function Team({ data, onAction }: { data: any; onAction: (name: string) => void }) {
  return (
    <Page>
      <Row variant="page-header">
        <Stack gap="sm">
          <Text variant="eyebrow">PEOPLE & PERMISSIONS</Text>
          <Heading level={1}>Your Hosting team</Heading>
          <Text variant="muted">Identity belongs to MyAccounts. Hosting permissions stay explicit.</Text>
        </Stack>
      </Row>
      <Alert tone="info">{data.teamGuidance}</Alert>
      {data.team.map((item, i) => (
        <Card key={i} href={item.href} variant="member">
          <Row>
            <Icon name="UserRound" />
            <Stack gap="xs">
              <Heading level={3}>{item.name}</Heading>
              <Text variant="caption">{item.email}</Text>
              <Text variant="caption">{item.identity}</Text>
            </Stack>
            <Spacer />
            <Badge tone={item.tone}>{item.role}</Badge>
          </Row>
        </Card>
      ))}
      {data.selectedUser && (
        <Card variant="form">
          <Heading level={2}>{data.selectedUser.name}</Heading>
          <Text variant="muted">Global administrators can configure and deploy every project. Viewers require explicit project membership. Only the configured Workspace owner changes global roles.</Text>
          <Field label="Global role"><Select name="globalRole" options={data.globalRoleOptions} /></Field>
          {data.isOwner && (
            <Button onClick={() => onAction("saveGlobalRole")}>Save global role</Button>
          )}
        </Card>
      )}
    </Page>
  )
}

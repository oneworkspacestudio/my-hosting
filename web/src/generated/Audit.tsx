// Generated from screens/Audit.aui; edit the AUI source.
import { Alert, Badge, Card, Heading, Page, Row, Stack, Text } from "@/components/ui"

export function Audit({ data, onAction }: { data: any; onAction: (name: string) => void }) {
  return (
    <Page>
      <Row variant="page-header">
        <Stack gap="sm">
          <Text variant="eyebrow">WORKSPACE SECURITY</Text>
          <Heading level={1}>Audit trail</Heading>
          <Text variant="muted">The latest 100 append-only Hosting events.</Text>
        </Stack>
      </Row>
      <Alert tone="info">Events have a SHA-256 chain and cannot be edited or deleted through the API. Administrators with direct database access remain trusted.</Alert>
      {data.audit.map((item, i) => (
        <Card key={i} variant="event">
          <Row>
            <Badge tone="muted">{item.action}</Badge>
            <Text variant="caption">{item.at}</Text>
          </Row>
          <Text variant="code">{item.summary}</Text>
        </Card>
      ))}
    </Page>
  )
}

# Matomo SPA pageviews

Agoraform can configure Matomo Tag Manager to record both the initial page load
and client-side navigation in a single-page application without editing router
code when the application uses the browser History API.

## Recommended configuration

Use separate triggers and tags for the initial page load and route changes:

```yaml
resources:
  - address: matomo.trigger.pageview
    attributes:
      type: pageView

  - address: matomo.trigger.route_change
    attributes:
      type: historyChange
      historySource: pushState

  - address: matomo.tag.pageview
    attributes:
      type: matomoAnalytics
      trackingType: pageview
      trigger:
        $ref: matomo.trigger.pageview
      documentTitle: "{{PageTitle}}"
      customUrl: "{{PageUrl}}"

  - address: matomo.tag.route_change
    attributes:
      type: matomoAnalytics
      trackingType: pageview
      name: SPA route change
      trigger:
        $ref: matomo.trigger.route_change
      documentTitle: "{{PageTitle}}"
      customUrl: "{{PageUrl}}"
```

`pageView` fires for the initial container page view. `historyChange` fires when
Matomo observes `pushState`, `replaceState`, `hashchange`, or `popstate`.
History Change does not replace the initial Pageview trigger.

## Avoid duplicate route pageviews

Some routers emit more than one History Change source during a single logical
navigation. For example, a route transition can call `pushState` and then
`replaceState`. An unfiltered History Change trigger can therefore fire the
route-change pageview tag more than once.

Set optional `historySource` when the application has one source that
represents its logical navigation:

```yaml
- address: matomo.trigger.route_change
  attributes:
    type: historyChange
    historySource: pushState
```

Supported values are:

- `pushState`
- `replaceState`
- `hashchange`
- `popstate`

Agoraform translates that field to Matomo's native trigger condition:
`HistorySource equals <value>`. Import reconstructs the field from that exact
condition, and updates replace only the managed History Source condition while
preserving unrelated trigger conditions.

Omit `historySource` only when each logical route transition produces one
History Change event, or when intentionally tracking all History API sources.
Use Matomo preview/debug mode to confirm which source your router emits before
choosing the filter.

## URL and title

For path-based routing, `customUrl: "{{PageUrl}}"` is normally appropriate. For
hash routing, use the URL template that represents the route you want Matomo to
record. Keep `document.title` current before the pageview tag fires when using
`{{PageTitle}}`.

## Application boundary

Native `pageView` and `historyChange` triggers are Tag Manager configuration,
not `applicationEvents`. Agoraform does not modify Next.js, React, Astro, or
other router code. If an application does not expose route changes through the
History API, use an application-owned event and a managed `customEvent` trigger
instead.

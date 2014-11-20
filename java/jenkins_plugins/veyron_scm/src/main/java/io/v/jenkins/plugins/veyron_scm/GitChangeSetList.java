package io.v.jenkins.plugins.veyron_scm;

import org.kohsuke.stapler.export.Exported;

import hudson.model.AbstractBuild;
import hudson.scm.ChangeLogSet;

import java.util.Collections;
import java.util.Iterator;
import java.util.List;


/**
 * List of changeset that went into a particular build.
 *
 * @author Nigel Magnay
 */
public class GitChangeSetList extends ChangeLogSet<GitChangeSet> {
  private final List<GitChangeSet> changeSets;

  /* package */GitChangeSetList(AbstractBuild<?, ?> build, List<GitChangeSet> logs) {
    super(build);
    Collections.reverse(logs); // put new things first
    this.changeSets = Collections.unmodifiableList(logs);
    for (GitChangeSet log : logs)
      log.setParent(this);
  }

  @Override
  public boolean isEmptySet() {
    return changeSets.isEmpty();
  }

  public Iterator<GitChangeSet> iterator() {
    return changeSets.iterator();
  }

  public List<GitChangeSet> getLogs() {
    return changeSets;
  }

  @Override
  @Exported
  public String getKind() {
    return "git";
  }

}

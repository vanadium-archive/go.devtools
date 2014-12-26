package io.v.jenkins.plugins.vanadium_scm;

import hudson.Functions;
import hudson.model.Action;
import hudson.model.AbstractBuild;
import io.v.jenkins.plugins.vanadium_scm.VanadiumSCM.ProjectSnapshot;

import java.util.Collections;
import java.util.Comparator;
import java.util.List;

/**
 * Captures the Vanadium related information for a build.
 *
 * <p>
 * This object is added to {@link AbstractBuild#getActions()}, and persists the Vanadium related
 * information of that build.
 */
public class VanadiumBuildData implements Action {

  private String curBuildCop;

  private List<ProjectSnapshot> snapshots;

  public VanadiumBuildData(String curBuildCop, List<ProjectSnapshot> snapshots) {
    this.curBuildCop = curBuildCop;
    this.snapshots = snapshots;

    // Sort by short project names.
    Collections.sort(snapshots, new Comparator<ProjectSnapshot>() {
      @Override
      public int compare(ProjectSnapshot o1, ProjectSnapshot o2) {
        return o1.getShortName().compareTo(o2.getShortName());
      }
    });
  }

  public String getCurBuildCop() {
    return curBuildCop;
  }

  public List<ProjectSnapshot> getSnapshots() {
    return snapshots;
  }

  @Override
  public String getIconFileName() {
    return Functions.getResourcePath() + "/plugin/vanadium_scm/icons/v-48x48.png";
  }

  @Override
  public String getDisplayName() {
    return "Vanadium Build Data";
  }

  @Override
  public String getUrlName() {
    return "vanadium";
  }
}
